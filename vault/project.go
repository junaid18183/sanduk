package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// ProjectFileName is a committed, non-secret file that names the vault entry a
// repository uses, so its commands can run without repeating the entry name.
//
// It must never contain secret values — only the entry name and, optionally,
// the subset of keys the project needs by default.
const ProjectFileName = ".sanduk.toml"

// Project is the contents of ProjectFileName.
type Project struct {
	// Entry is the vault entry the repository resolves to by default.
	Entry string `toml:"entry"`
	// Only optionally narrows the default entry to a subset of its keys.
	Only []string `toml:"only,omitempty"`
	// Include names entries merged underneath Entry: the global secrets and
	// groups this repository also needs. They resolve before Entry, in the
	// order given, so the project's own entry wins on a variable two of them
	// both provide — the same left-to-right, last-wins rule as
	// "sanduk exec a b c".
	//
	// Like Entry these are names, never values, and the whole file is
	// approved as one: a repository cannot widen what it receives without
	// the change showing up in "sanduk allow".
	Include []string `toml:"include,omitempty"`
	// Inherit reports whether the project files above this one apply here
	// too. It defaults to true: a directory tree is a hierarchy, and the
	// usual reason to put a project file above a repository is to share
	// something with everything under it.
	//
	// Setting it to false seals the directory. A repository that must
	// resolve to the same variables in a clone and in CI wants that, because
	// a file above the repository root is not in the clone — see Inherits.
	Inherit *bool `toml:"inherit,omitempty"`

	// path and hash identify the exact bytes this Project was parsed from.
	// Approval is checked against hash rather than by reading the file again,
	// so a file swapped between the parse and the check cannot pass as the
	// approved one.
	path string
	hash string
}

// Hash is the sha256 of the bytes this Project was parsed from, which is what
// approval is recorded against. A caller rewriting the file compares it with
// a fresh read, so an edit made by somebody else in between is not clobbered.
func (p *Project) Hash() string { return p.hash }

// Path is the project file this Project was parsed from.
func (p *Project) Path() string { return p.path }

// Dir is the directory the project file lives in: what a reader recognises,
// and what a recorded group location holds.
func (p *Project) Dir() string { return filepath.Dir(p.path) }

// Inherits reports whether the project files above this one also apply.
//
// The layers above a repository root are not in the clone, so what they
// contribute is a convenience on this machine and nowhere else. That is the
// point of the setting: a repository whose "sanduk exec" must behave the same
// in CI writes inherit = false and depends on its own file alone.
func (p *Project) Inherits() bool { return p.Inherit == nil || *p.Inherit }

// Entries returns every entry the project resolves, in resolution order —
// the includes first, then Entry — so a caller can hand them straight to the
// selector pipeline.
func (p *Project) Entries() []string {
	out := make([]string, 0, len(p.Include)+1)
	out = append(out, p.Include...)
	return append(out, p.Entry)
}

// LoadProject walks up from dir and returns the nearest ProjectFileName.
//
// It returns ("", nil, nil) when no project file exists anywhere above dir, so
// callers can treat "no project" as a normal state rather than an error.
func LoadProject(dir string) (string, *Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	for {
		p, err := loadProjectFile(filepath.Join(abs, ProjectFileName))
		if err != nil {
			return "", nil, err
		}
		if p != nil {
			return p.path, p, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", nil, nil
		}
		abs = parent
	}
}

// LoadProjectChain returns every project file that applies to dir, outermost
// first — the order they resolve in, so the nearest one is last and wins.
//
// A directory tree is a hierarchy: ~/code can hold the token every checkout
// below it uses, ~/code/acme the one its repositories use instead, and each
// repository its own entry. Each file is a separate statement, approved on
// its own, and each narrows the one above by the same last-wins rule as
// "sanduk exec a b c".
//
// The walk stops at the first file that sets inherit = false, which is
// included: sealing a directory means nothing above it applies, not that the
// file itself is skipped. It returns nil when there is no project file above
// dir at all.
//
// Nothing here checks approval. That is the caller's job, because a listing
// shows an unapproved layer and says so while exec refuses outright.
func LoadProjectChain(dir string) ([]*Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var chain []*Project
	for {
		p, err := loadProjectFile(filepath.Join(abs, ProjectFileName))
		if err != nil {
			return nil, err
		}
		if p != nil {
			chain = append(chain, p)
			if !p.Inherits() {
				break
			}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}

	// Collected nearest first; resolution runs the other way.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// loadProjectFile parses one project file, or returns (nil, nil) when there is
// none at path. A file that exists but does not parse is an error: a project
// file sanduk cannot read must never be silently skipped, because skipping it
// would quietly resolve something other than what the directory asked for.
func loadProjectFile(path string) (*Project, error) {
	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return nil, nil
	case err != nil:
		return nil, err
	}
	if err := checkProjectOwner(path); err != nil {
		return nil, err
	}
	var p Project
	if err := toml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if p.Entry == "" {
		return nil, fmt.Errorf("%s: entry is required", path)
	}
	if err := ValidateEntryName(p.Entry); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, inc := range p.Include {
		if err := ValidateEntryName(inc); err != nil {
			return nil, fmt.Errorf("%s: include: %w", path, err)
		}
		// include takes entry names, not exec's ENVVAR=entry bindings: a
		// binding here would be a second syntax to learn in a file whose job
		// is to name entries.
		if strings.Contains(inc, "=") {
			return nil, fmt.Errorf("%s: include %q: names only, not a VAR=entry binding", path, inc)
		}
	}
	p.path = path
	p.hash = hashBytes(b)
	return &p, nil
}

// FormatOnly renders an only list as it is written in a project file.
func FormatOnly(keys []string) string {
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = strconv.Quote(k)
	}
	return "only = [" + strings.Join(quoted, ", ") + "]"
}

// ReplaceOnly returns src with its `only` assignment rewritten to keys, and
// reports whether it found one.
//
// The file is edited in place rather than re-encoded, because it is a file a
// human wrote and committed: re-marshalling it would silently drop the
// comments explaining what it is for and reorder everything else. Every byte
// outside the assignment is preserved, and a file with no `only` is left
// alone — sanduk keeps such a list in step, it does not start one.
//
// An assignment may span lines, so the scan runs from the line that opens it
// to the line that closes the bracket.
func ReplaceOnly(src []byte, keys []string) ([]byte, bool) {
	lines := strings.Split(string(src), "\n")
	start := -1
	for i, line := range lines {
		if isOnlyAssignment(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return src, false
	}
	end := start
	for !strings.Contains(lines[end], "]") {
		end++
		if end == len(lines) {
			// An unterminated list: the file is not what we think it is, so
			// leave it untouched rather than guess where the list ends.
			return src, false
		}
	}

	indent := lines[start][:len(lines[start])-len(strings.TrimLeft(lines[start], " \t"))]
	out := append([]string{}, lines[:start]...)
	out = append(out, indent+FormatOnly(keys))
	out = append(out, lines[end+1:]...)
	return []byte(strings.Join(out, "\n")), true
}

// isOnlyAssignment reports whether a line opens the top-level `only` key. A
// commented-out line is not one, and neither is a key that merely ends in
// "only".
func isOnlyAssignment(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, "only") {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(t[len("only"):], " \t"), "=")
}
