//go:build !windows

package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustStoreWithLoosePermissionsIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ProjectFileName), "entry = \"warden\"\n")
	tp := filepath.Join(t.TempDir(), TrustFileName)

	p := loadTestProject(t, dir)
	if _, err := ApproveProject(tp, p); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}
	// A trust store git checked out, or one someone loosened, has group/other
	// bits set; sanduk never writes it that way.
	if err := os.Chmod(tp, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := IsProjectTrusted(tp, p); err == nil {
		t.Fatal("trusted a world-readable trust store")
	}
}

func TestProjectFileWorldWritableIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectFileName)
	writeTestFile(t, path, "entry = \"warden\"\n")
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadProject(dir); err == nil {
		t.Fatal("loaded a world-writable project file")
	}
}
