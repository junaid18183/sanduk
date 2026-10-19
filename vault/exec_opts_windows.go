//go:build windows

package vault

import "os"

// resolve materializes the file secrets into a directory and returns a
// cleanup to remove them once the child has exited. Windows has no /dev/fd,
// so there is always a real file; keeping it inside a private directory that
// this process deletes is the closest equivalent.
func (o ExecOptions) resolve() (map[string]string, func(), error) {
	env := copyEnv(o.Env)
	noop := func() {}
	if len(o.Files) == 0 {
		return env, noop, nil
	}
	if o.MaterializeDir != "" {
		paths, err := MaterializeInto(o.MaterializeDir, o.Files)
		if err != nil {
			return nil, noop, err
		}
		merged, _, err := mergePaths(env, paths)
		return merged, noop, err
	}
	dir, err := os.MkdirTemp("", "sanduk-")
	if err != nil {
		return nil, noop, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	paths, err := MaterializeInto(dir, o.Files)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	merged, _, err := mergePaths(env, paths)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	return merged, cleanup, nil
}
