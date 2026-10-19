//go:build windows

package vault

// checkProjectOwner is a no-op on Windows, which has no uid/mode bits to check;
// approval (sanduk allow) is the only gate there.
func checkProjectOwner(path string) error { return nil }

// checkPrivateFile is a no-op on Windows; see checkProjectOwner.
func checkPrivateFile(path string) error { return nil }
