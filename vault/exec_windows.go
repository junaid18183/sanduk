//go:build windows

package vault

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Exec runs command as a child process, injecting env as environment
// variables. Windows has no execve, so the parent waits for the child.
func Exec(name string, args []string, env map[string]string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = buildEnv(env)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("could not run %q: %w", name, err)
	}
	return nil
}
