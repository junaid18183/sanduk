//go:build !windows

package vault

import (
	"os"
	"path/filepath"
	"syscall"
)

// fileLock is an advisory, cross-process exclusive lock backed by flock(2). The
// per-process mutex in the age backend stops two goroutines from clobbering each
// other, but only an OS lock stops two concurrent `sanduk add` invocations from
// losing one of the updates.
type fileLock struct{ f *os.File }

// lockPath takes an exclusive lock on "<path>.lock", creating the lock file and
// its directory if needed. The lock is released by release, and the lock file is
// left in place: removing a file other processes may be about to lock is racy.
func lockPath(path string) (*fileLock, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}
