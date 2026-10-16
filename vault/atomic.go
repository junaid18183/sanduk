package vault

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path with perm, via a temporary file in the
// same directory that is fsynced and then renamed. A crash can therefore never
// leave a truncated file, and a concurrent reader always sees either the old or
// the new complete contents.
//
// It is used for the metadata config, the trust store, and the age vault, so
// none of them can be corrupted by an interrupted write.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	// Best-effort directory fsync so the rename itself survives a crash. Not
	// every filesystem supports it, so a failure here is not fatal.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
