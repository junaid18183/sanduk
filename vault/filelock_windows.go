//go:build windows

package vault

// fileLock is a no-op on Windows. The default backend there is Windows
// Credential Manager, which serialises writes itself; the age file backend is
// not the default, and cross-process locking for it is a known limitation.
type fileLock struct{}

func lockPath(string) (*fileLock, error) { return &fileLock{}, nil }

func (l *fileLock) release() {}
