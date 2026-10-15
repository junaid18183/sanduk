//go:build !windows

package vault

import (
	"fmt"
	"os/exec"
	"syscall"
)

// Exec replaces the current process with command, injecting env as
// environment variables. This means secrets are visible only to the child
// process, never to the caller's shell.
func Exec(name string, args []string, env map[string]string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("could not find %q in PATH", name)
	}
	argv := append([]string{name}, args...)
	return syscall.Exec(path, argv, buildEnv(env))
}
