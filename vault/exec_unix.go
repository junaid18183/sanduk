//go:build !windows

package vault

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
)

// Exec replaces the current process with command, injecting opts.Env as
// environment variables. This means secrets are visible only to the child
// process, never to the caller's shell.
//
// File secrets are materialized first. By default they become unlinked
// descriptors the child inherits as /dev/fd/N, so nothing readable is left on
// disk; with opts.MaterializeDir they become real 0600 files there, which
// outlive the command because execve leaves nobody behind to delete them.
func Exec(name string, args []string, opts ExecOptions) error {
	env, keep, err := opts.resolve()
	if err != nil {
		return err
	}
	if err := validateEnvNames(env); err != nil {
		return err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("could not find %q in PATH", name)
	}
	argv := append([]string{name}, args...)

	// The descriptors behind /dev/fd/N must still be open when execve runs.
	defer runtime.KeepAlive(keep)
	return syscall.Exec(path, argv, buildEnv(env))
}
