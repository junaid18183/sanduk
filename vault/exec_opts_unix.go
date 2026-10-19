//go:build !windows

package vault

import "os"

// resolve materializes the file secrets and merges their paths into a copy of
// the environment. The returned files must stay alive until execve; see
// materializeEphemeral.
func (o ExecOptions) resolve() (map[string]string, []*os.File, error) {
	env := copyEnv(o.Env)
	if len(o.Files) == 0 {
		return env, nil, nil
	}
	if o.MaterializeDir != "" {
		paths, err := MaterializeInto(o.MaterializeDir, o.Files)
		if err != nil {
			return nil, nil, err
		}
		merged, _, err := mergePaths(env, paths)
		return merged, nil, err
	}
	paths, keep, err := materializeEphemeral(o.Files)
	if err != nil {
		return nil, nil, err
	}
	merged, _, err := mergePaths(env, paths)
	return merged, keep, err
}
