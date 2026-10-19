//go:build !windows

package vault

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// materializeEphemeral writes each payload to a temporary file, unlinks it at
// once, and returns /dev/fd/N for the descriptor the child will inherit. The
// bytes therefore have no name on disk for any other process to open, and the
// kernel frees them when the last descriptor closes — which, since exec
// replaces this process, is when the child exits.
//
// The returned files must stay reachable until execve: closing one, or letting
// the garbage collector finalize it, would close the descriptor the path
// refers to.
func materializeEphemeral(files map[string]FilePayload) (map[string]string, []*os.File, error) {
	paths := make(map[string]string, len(files))
	open := make([]*os.File, 0, len(files))

	closeAll := func() {
		for _, f := range open {
			f.Close()
		}
	}

	for _, envName := range sortedPayloadNames(files) {
		f, err := os.CreateTemp("", "sanduk-")
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		// Unlink before writing: if anything below fails, there is no file
		// left behind holding the secret.
		if err := os.Remove(f.Name()); err != nil {
			f.Close()
			closeAll()
			return nil, nil, err
		}
		if _, err := f.Write(files[envName].Data); err != nil {
			f.Close()
			closeAll()
			return nil, nil, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			closeAll()
			return nil, nil, err
		}
		// Go opens files close-on-exec. Clear that, or the descriptor the
		// path names would be gone the moment the child starts.
		fd := f.Fd()
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, uintptr(syscall.F_SETFD), 0); errno != 0 {
			f.Close()
			closeAll()
			return nil, nil, fmt.Errorf("keeping the file secret descriptor open across exec: %w", errno)
		}

		open = append(open, f)
		paths[envName] = fmt.Sprintf("/dev/fd/%d", fd)
	}
	return paths, open, nil
}
