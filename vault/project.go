package vault

import (
	"fmt"
	"os"
	"path/filepath"

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

	// path and hash identify the exact bytes this Project was parsed from.
	// Approval is checked against hash rather than by reading the file again,
	// so a file swapped between the parse and the check cannot pass as the
	// approved one.
	path string
	hash string
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
		path := filepath.Join(abs, ProjectFileName)
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := checkProjectOwner(path); err != nil {
				return "", nil, err
			}
			var p Project
			if err := toml.Unmarshal(b, &p); err != nil {
				return "", nil, fmt.Errorf("%s: %w", path, err)
			}
			if p.Entry == "" {
				return "", nil, fmt.Errorf("%s: entry is required", path)
			}
			p.path = path
			p.hash = hashBytes(b)
			return path, &p, nil
		case !os.IsNotExist(err):
			return "", nil, err
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return "", nil, nil
		}
		abs = parent
	}
}
