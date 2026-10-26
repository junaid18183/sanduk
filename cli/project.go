package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/junaid18183/sanduk/vault"
)

// defaultEntry resolves the entries a repository injects by default: the
// entry named by .sanduk.toml, the entries it includes underneath, and its
// optional key subset. It fails with an actionable message
// when there is no project file, so commands that can omit the entry say why
// they cannot.
//
// The project file must be approved first. A cloned repository can otherwise
// commit an entry name of its choosing, and "sanduk exec -- <its scripts>" would
// hand that entry's secrets to the repository. Approval is a hash of the file's
// contents, so editing it requires re-approving it.
func (a *app) defaultEntry() (entries []string, only []string, err error) {
	path, proj, err := vault.LoadProject(".")
	if err != nil {
		return nil, nil, err
	}
	if proj == nil {
		return nil, nil, fmt.Errorf("no entry given and no %s found in this directory or its parents",
			vault.ProjectFileName)
	}
	if err := a.requireTrustedProject(path, proj); err != nil {
		return nil, nil, err
	}
	// Includes first, the project's own entry last: the same order, and the
	// same last-wins rule, as naming them on the command line.
	return proj.Entries(), proj.Only, nil
}

// projectScope resolves the entry the working directory belongs to, for
// display only.
//
// Two things can claim a directory, and they are not the same statement:
//
//   - .sanduk.toml is the repository's claim, committed for everyone who
//     clones it. It is portable and it is untrusted, so it counts only once
//     approved.
//   - a group's location is your own machine's note, written by add-group or
//     allow into your private config. Nothing a repository ships can forge
//     it, so it needs no approval — but it does not travel.
//
// The approved project file wins, so what a listing shows is what a teammate
// and CI resolve from the same repository; the recorded location is the
// fallback, which keeps a group visible in its own directory after
// add-group --no-project, or when the project file was never committed.
//
// This decides a view and nothing else. exec, export and doctor still resolve
// their default entry through defaultEntry, from the project file alone, so a
// location can never make a command inject something a teammate's checkout
// would not.
//
// Unlike defaultEntry this never fails the command: a listing is not an
// injection, so an unusable project file only costs the narrower view. The
// reason comes back separately, because a repository that silently failed to
// narrow the output would read as sanduk ignoring it.
func (a *app) projectScope(cfg *vault.Config) (origin, entry string, why error) {
	path, proj, err := vault.LoadProject(".")
	switch {
	case err != nil:
		why = err
	case proj != nil:
		if terr := a.requireTrustedProject(path, proj); terr != nil {
			why = terr
		} else {
			// The directory, not the file: it is what the reader
			// recognises, and what a recorded location holds.
			return filepath.Dir(path), proj.Entry, nil
		}
	}
	if name, ok := cfg.GroupAt("."); ok {
		return cfg.Groups[name].Location, name, why
	}
	if path != "" {
		path = filepath.Dir(path)
	}
	return path, "", why
}

// repoRoot returns the nearest repository root at or above dir, and whether
// that root is dir itself.
//
// A group is tied to a directory only when that directory is obviously a
// project, so the caller acts on here and only mentions a root further up.
// Adopting an ancestor would mean a command run deep in a tree writing a file
// into a repository the user is not looking at. The .git entry is a file in a
// worktree or submodule, so its kind is not checked.
func repoRoot(dir string) (root string, here bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for cur := abs; ; {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur, cur == abs
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// adoptProject records group as the group belonging to dir and, when the
// directory has no project file yet, writes one and approves it.
//
// Approving what sanduk just wrote is not a hole in the approval rule: that
// rule exists because a repository can commit a project file naming any entry
// it likes, and these bytes came from this command, on this machine, naming
// the group the user is creating right now. Editing the file afterwards still
// revokes the approval, exactly as for any other project file.
func (a *app) adoptProject(group, dir string) error {
	var recorded bool
	if err := a.updateConfig(func(c *vault.Config) {
		recorded = c.SetGroupLocation(group, dir)
	}); err != nil {
		return err
	}
	if !recorded {
		return nil
	}
	a.warnf("Recorded %q as the group for %s\n", group, dir)

	path := filepath.Join(dir, vault.ProjectFileName)
	switch _, err := os.Stat(path); {
	case err == nil:
		a.warnf("%s already exists; left unchanged\n", path)
		return nil
	case !os.IsNotExist(err):
		return err
	}
	// 0644: the file is committed and carries no value. LoadProject still
	// refuses it if it is world-writable or owned by somebody else.
	if err := vault.WriteFileAtomic(path, initFileBytes(group, nil, nil), 0o644); err != nil {
		return err
	}
	_, proj, err := vault.LoadProject(dir)
	if err != nil {
		return err
	}
	if proj == nil {
		return fmt.Errorf("wrote %s but could not read it back", path)
	}
	if _, err := vault.ApproveProject(a.trustPath(), proj); err != nil {
		return err
	}
	a.warnf("Wrote and approved %s — commit it: it names the entry, never a value\n", path)
	return nil
}

// requireTrustedProject refuses an unapproved (or edited) project file and tells
// the user how to approve it.
func (a *app) requireTrustedProject(path string, proj *vault.Project) error {
	ok, err := vault.IsProjectTrusted(a.trustPath(), proj)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not approved; review it, then run \"sanduk allow\" to trust entry %q",
			path, proj.Entry)
	}
	return nil
}

// syncProjectOnly keeps a project file's `only` list in step with the group it
// names, adding the keys an edit introduced and dropping the ones it removed.
//
// It is deliberately conservative, because it edits a file the user commits:
//
//   - a file with no `only` is left alone. sanduk keeps such a list current,
//     it does not start one, so deleting the line opts out for good.
//   - a file that is not approved is left alone. Editing it would mean
//     approving it afterwards, and approval is the user's decision, not a
//     side effect of storing a field.
//   - only the assignment is rewritten; every other byte, comments included,
//     is preserved.
//
// The rewrite changes the file's hash, which revokes its approval, so the new
// bytes are approved again — they came from this command, and the list now
// says exactly what the group holds.
//
// A failure here never fails the edit the user asked for: the secret is
// already stored, and a stale list is reported by doctor, so the error
// becomes a note.
func (a *app) syncProjectOnly(group string, add, remove []string) {
	cfg, err := a.loadConfig()
	if err != nil {
		return
	}
	dir := a.projectDirFor(cfg, group)
	if dir == "" {
		return
	}
	path, proj, err := vault.LoadProject(dir)
	if err != nil || proj == nil || proj.Entry != group || len(proj.Only) == 0 {
		return
	}
	if ok, err := vault.IsProjectTrusted(a.trustPath(), proj); err != nil || !ok {
		return
	}

	next := mergeOnly(proj.Only, add, remove)
	if equalStrings(next, proj.Only) {
		return
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// Rewrite only the bytes that were parsed and approved. If the file
	// changed in between, that is somebody else's edit and the list computed
	// from the old contents would clobber it.
	if vault.HashBytes(src) != proj.Hash() {
		a.warnf("note: %s changed while sanduk was writing; only was left alone\n", path)
		return
	}
	out, ok := vault.ReplaceOnly(src, next)
	if !ok {
		a.warnf("note: could not update only in %s; run \"sanduk doctor\" to check it\n", path)
		return
	}
	if err := vault.WriteFileAtomic(path, out, 0o644); err != nil {
		a.warnf("note: could not write %s: %v\n", path, err)
		return
	}
	_, reloaded, err := vault.LoadProject(dir)
	if err != nil || reloaded == nil {
		return
	}
	if _, err := vault.ApproveProject(a.trustPath(), reloaded); err != nil {
		a.warnf("note: %s was updated but not re-approved: %v\n", path, err)
		return
	}
	a.warnf("Updated only in %s (%d key(s))\n", path, len(next))
}

// projectDirFor is the directory whose project file names group: the location
// recorded for it, or the working directory when its project file names it.
func (a *app) projectDirFor(cfg *vault.Config, group string) string {
	if loc := cfg.Groups[group].Location; loc != "" {
		return loc
	}
	_, proj, err := vault.LoadProject(".")
	if err != nil || proj == nil || proj.Entry != group {
		return ""
	}
	return "."
}

// mergeOnly applies an edit to an only list, keeping the order the file had so
// the diff stays small, and appending what is new in sorted order.
func mergeOnly(current, add, remove []string) []string {
	drop := make(map[string]bool, len(remove))
	for _, k := range remove {
		drop[k] = true
	}
	have := make(map[string]bool, len(current))
	out := make([]string, 0, len(current)+len(add))
	for _, k := range current {
		if drop[k] || have[k] {
			continue
		}
		have[k] = true
		out = append(out, k)
	}
	var added []string
	for _, k := range add {
		if drop[k] || have[k] {
			continue
		}
		have[k] = true
		added = append(added, k)
	}
	sort.Strings(added)
	return append(out, added...)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
