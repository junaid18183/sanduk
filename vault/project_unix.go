//go:build !windows

package vault

import (
	"fmt"
	"os"
	"syscall"
)

// checkProjectOwner refuses a project file that another user owns or that any
// user can write. The lookup walks up to the filesystem root, so without this a
// .sanduk.toml planted in a shared directory such as /tmp would be picked up by
// every repository below it.
func checkProjectOwner(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o002 != 0 {
		return fmt.Errorf("%s is world-writable; refusing to use it", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uid := os.Getuid(); int(st.Uid) != uid && st.Uid != 0 {
		return fmt.Errorf("%s is owned by another user (uid %d); refusing to use it", path, st.Uid)
	}
	return nil
}

// checkPrivateFile refuses a file that sanduk itself writes with mode 0600 but
// that is now readable or writable by group or others, or that another user
// owns (unless running as root). A trust store checked out
// by git (mode 0644) fails this, which is the point: sanduk never creates one
// that way.
func checkPrivateFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s has mode %#o; sanduk writes it as 0600 — fix with: chmod 600 %s", path, perm, path)
	}
	// Root can already read and rewrite any user's files, so refusing it
	// protects nothing and only breaks `sudo sanduk` against a user's config.
	if uid := os.Getuid(); uid != 0 {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
			return fmt.Errorf("%s is owned by another user (uid %d)", path, st.Uid)
		}
	}
	return nil
}
