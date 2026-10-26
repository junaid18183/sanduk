package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
)

// projectLayer is one directory of the chain that claims the working
// directory, as a listing shows it.
type projectLayer struct {
	// Dir is the directory the layer speaks for.
	Dir string
	// Entry is the entry it names, and Include the entries merged underneath
	// that one, in resolution order.
	Entry   string
	Include []string
	// Only narrows Entry, never the includes — see resolveProjects.
	Only []string
	// Trusted is false for a project file that has not been approved. Such a
	// layer injects nothing: it is carried so a listing can say why, which
	// is what Why holds.
	Trusted bool
	Why     error
	// FromLocation marks a layer that came from the group location recorded
	// in your own config rather than from a project file.
	FromLocation bool
}

// entries returns what the layer resolves, in order: its includes, then its
// own entry, which therefore wins on a variable two of them provide.
func (l projectLayer) entries() []string {
	out := make([]string, 0, len(l.Include)+1)
	out = append(out, l.Include...)
	return append(out, l.Entry)
}

// defaultProjects resolves the chain of project files that applies to the
// working directory, outermost first, for a command that was given no entry.
//
// Every layer must be approved. A cloned repository can otherwise commit a
// project file naming an entry of its choosing, and "sanduk exec -- <its
// scripts>" would hand that entry's secrets to the repository; a file above
// the repository is no different, since a directory you cloned into can hold
// one too. An unapproved layer anywhere in the chain fails the command and
// names the directory to approve, because the alternative — resolving the
// rest and carrying on — is a run that is quietly missing a variable.
func (a *app) defaultProjects() ([]projectLayer, error) {
	chain, err := vault.LoadProjectChain(".")
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("no entry given and no %s found in this directory or its parents",
			vault.ProjectFileName)
	}
	out := make([]projectLayer, 0, len(chain))
	for _, proj := range chain {
		if err := a.requireTrustedProject(proj); err != nil {
			return nil, err
		}
		out = append(out, projectLayer{
			Dir: proj.Dir(), Entry: proj.Entry, Include: proj.Include,
			Only: proj.Only, Trusted: true,
		})
	}
	return out, nil
}

// projectChain resolves the layers the working directory belongs to, for
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
// The approved project files come first, outermost to nearest, because that
// is the order they inject in. A recorded location is added underneath them
// only when it is deeper than every one of them, which keeps a group visible
// in its own directory after add-group --no-project, or when the project
// file was never committed.
//
// This decides a view and nothing else. exec, export and doctor still resolve
// through defaultProjects, from project files alone, so a location can never
// make a command inject something a teammate's checkout would not.
//
// Unlike defaultProjects this never fails the command: a listing is not an
// injection, so an unusable or unapproved layer only costs the narrower view.
// The reason comes back separately, because a repository that silently failed
// to narrow the output would read as sanduk ignoring it.
func (a *app) projectChain(cfg *vault.Config) (layers []projectLayer, why error) {
	chain, err := vault.LoadProjectChain(".")
	if err != nil {
		why = err
	}
	for _, proj := range chain {
		l := projectLayer{
			Dir: proj.Dir(), Entry: proj.Entry, Include: proj.Include,
			Only: proj.Only, Trusted: true,
		}
		if terr := a.requireTrustedProject(proj); terr != nil {
			l.Trusted, l.Why = false, terr
		}
		layers = append(layers, l)
	}

	// A recorded location is added only where no project file already speaks
	// for the same directory or a deeper one: with nothing above it that is
	// the whole view, and underneath a chain it is the nearer statement. A
	// location further up never displaces the files, which are the portable
	// claim and the one a teammate's checkout resolves.
	if name, ok := cfg.GroupAt("."); ok && !claimsEntry(layers, name) {
		dir := cfg.Groups[name].Location
		deepest := innermostDir(layers)
		if deepest == "" || (dir != deepest && withinDir(deepest, dir)) {
			layers = append(layers, projectLayer{
				Dir: dir, Entry: name, Trusted: true, FromLocation: true,
			})
		}
	}
	return layers, why
}

// warnUntrusted says, for every layer that is not approved, what it would
// have contributed and how to approve it. Each file is a separate decision,
// so each gets its own line rather than only the first one found.
func (a *app) warnUntrusted(layers []projectLayer) int {
	n := 0
	for _, l := range layers {
		if l.Why != nil {
			a.warnf("note: %v\n", l.Why)
			n++
		}
	}
	return n
}

// layerPaths renders a chain's directories for a table. The layers nest, so
// every path after the first repeats the one above it; printing each one in
// full turns the column into a wall of identical prefix. Only the part that
// differs is new information.
func layerPaths(layers []projectLayer) []string {
	out := make([]string, len(layers))
	for i, l := range layers {
		if i == 0 {
			out[i] = shortPath(l.Dir)
			continue
		}
		rel, err := filepath.Rel(layers[i-1].Dir, l.Dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			out[i] = shortPath(l.Dir)
			continue
		}
		out[i] = "…/" + rel
	}
	return out
}

// innermostDir is the directory of the last layer, or "" when there are none.
func innermostDir(layers []projectLayer) string {
	if len(layers) == 0 {
		return ""
	}
	return layers[len(layers)-1].Dir
}

// claimsEntry reports whether a layer already names the entry, so a recorded
// location never repeats what a project file has already said.
func claimsEntry(layers []projectLayer, name string) bool {
	for _, l := range layers {
		if l.Entry == name {
			return true
		}
	}
	return false
}

// withinDir reports whether path is root or lies under it.
func withinDir(root, path string) bool {
	if root == "" {
		return false
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// currentEntries lists every entry the layers resolve, for scope
// classification: each one injects here, so each is this directory's.
func currentEntries(layers []projectLayer) []string {
	var out []string
	for _, l := range layers {
		if l.Trusted {
			out = append(out, l.entries()...)
		}
	}
	return out
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
//
// The hint names the directory, because a file above the working directory is
// not the one a bare "sanduk allow" would approve.
func (a *app) requireTrustedProject(proj *vault.Project) error {
	ok, err := vault.IsProjectTrusted(a.trustPath(), proj)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not approved; review it, then run %s to trust entry %q",
			shortPath(proj.Path()), allowHint(proj.Dir()), proj.Entry)
	}
	return nil
}

// allowHint is the command that approves the project file in dir. "sanduk
// allow" approves the nearest file at or above the working directory, so a
// layer further up has to be named.
func allowHint(dir string) string {
	if wd, err := os.Getwd(); err == nil && wd == dir {
		return "\"sanduk allow\""
	}
	return "\"sanduk allow " + shortPath(dir) + "\""
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
