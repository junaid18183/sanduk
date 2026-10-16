package vault

import "fmt"

// ExitError asks the program to exit with Code. Library code returns it instead
// of calling os.Exit, so deferred cleanup runs and the caller decides how to
// exit. Err, when set, is the message to report; a nil Err (a child process's
// own exit status, for example) means there is nothing to add.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }
