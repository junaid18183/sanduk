package vault

// File secrets: credentials that a program insists on reading from a path
// rather than an environment variable — a service-account JSON, a .p8 key, a
// client certificate.
//
// The value still never touches the user's disk as a named file. exec writes
// the bytes to a temporary file, unlinks it immediately, and passes the child
// /dev/fd/N for the descriptor it inherits (see filesecret_unix.go), so the
// data has no path anyone else can open and is freed when the child exits.
// Windows has no equivalent, so it writes a real file into a private
// directory and removes it once the child is done.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExecOptions is everything Exec needs beyond the command itself.
type ExecOptions struct {
	// Env is the variables to inject.
	Env map[string]string
	// Files is the file secrets to materialize, keyed by the variable that
	// will hold each one's path.
	Files map[string]FilePayload
	// MaterializeDir, when set, writes file secrets there as real 0600 files
	// instead of the default ephemeral descriptors. The files persist after
	// the command: the caller asked for paths and owns removing them.
	MaterializeDir string
}

// copyEnv duplicates the caller's map so materialization never mutates it.
func copyEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

// mergePaths adds each materialized path to env under its variable name,
// refusing a name that a plain variable already occupies rather than letting
// one silently win.
func mergePaths(env, paths map[string]string) (map[string]string, []string, error) {
	added := make([]string, 0, len(paths))
	for name, path := range paths {
		if _, clash := env[name]; clash {
			return nil, nil, fmt.Errorf("%s is set by both a variable and a file secret", name)
		}
		env[name] = path
		added = append(added, name)
	}
	sort.Strings(added)
	return env, added, nil
}

// FilePayload is one file secret on its way to a child process.
type FilePayload struct {
	// Filename is the basename to give the materialized file. It is a hint
	// from the entry, never a path: see SafeFilename.
	Filename string
	Data     []byte
}

// SafeFilename returns the basename to write payload under, falling back to
// the variable name when the entry carries no hint. Any directory part is
// stripped: the filename comes out of the vault, and a stored "../../id_rsa"
// must not escape the directory sanduk chose.
func SafeFilename(envName string, p FilePayload) string {
	name := filepath.Base(strings.TrimSpace(p.Filename))
	if name == "." || name == string(filepath.Separator) || name == "" ||
		name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return strings.ToLower(envName)
	}
	return name
}

// MaterializeInto writes every payload into dir and returns the
// variable-to-path mapping. Files are created 0600 and the directory 0700
// where the OS has permission bits; Windows reports 0666 whatever is asked
// for, as it does for export --output. Unlike the exec path, these files
// outlive the command: the caller asked for real paths and owns removing them.
func MaterializeInto(dir string, files map[string]FilePayload) (map[string]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("no directory given for file secrets")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(files))
	used := map[string]bool{}
	for _, envName := range sortedPayloadNames(files) {
		p := files[envName]
		name := SafeFilename(envName, p)
		// Two entries may share a filename hint; never let one overwrite the
		// other's bytes.
		for n := 2; used[name]; n++ {
			ext := filepath.Ext(name)
			name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
		}
		used[name] = true

		path := filepath.Join(dir, name)
		if err := WriteFileAtomic(path, p.Data, 0o600); err != nil {
			return nil, err
		}
		out[envName] = path
	}
	return out, nil
}

func sortedPayloadNames(files map[string]FilePayload) []string {
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
