//go:build windows

package vault

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Exec runs command as a child process, injecting opts.Env as environment
// variables. Windows has no execve, so the parent waits for the child.
//
// That also means file secrets can be cleaned up here: Windows has no
// /dev/fd, so they are written into a private temporary directory which is
// removed once the child exits.
func Exec(name string, args []string, opts ExecOptions) error {
	env, cleanup, err := opts.resolve()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := validateEnvNames(env); err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Env = buildEnv(env)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Pass the child's status through, as execve does on Unix.
			return &ExitError{Code: exitErr.ExitCode()}
		}
		return fmt.Errorf("could not run %q: %w", name, err)
	}
	return nil
}
