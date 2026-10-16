package vault

import (
	"path/filepath"
	"testing"
)

func loadTestProject(t *testing.T, dir string) *Project {
	t.Helper()
	_, p, err := LoadProject(dir)
	if err != nil || p == nil {
		t.Fatalf("LoadProject: %v (project %v)", err, p)
	}
	return p
}

func TestProjectTrustLifecycle(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, ProjectFileName)
	writeTestFile(t, proj, "entry = \"warden\"\n")
	// Keep the trust store out of the developer's real config directory.
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	p := loadTestProject(t, dir)
	ok, err := IsProjectTrusted(TrustPath(), p)
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("an unapproved file must not be trusted")
	}

	if _, err := ApproveProject(TrustPath(), p); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}
	ok, err = IsProjectTrusted(TrustPath(), loadTestProject(t, dir))
	if err != nil || !ok {
		t.Fatalf("approved file should be trusted (ok=%v err=%v)", ok, err)
	}

	// Editing the file must revoke the approval.
	writeTestFile(t, proj, "entry = \"aws-prod\"\n")
	ok, err = IsProjectTrusted(TrustPath(), loadTestProject(t, dir))
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("editing the project file must revoke approval")
	}

	paths, err := TrustedPaths(TrustPath())
	if err != nil {
		t.Fatalf("TrustedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != p.Path() {
		t.Fatalf("TrustedPaths = %v, want [%s]", paths, p.Path())
	}
}

// The trust check must judge the bytes that were parsed. Swapping the file back
// to the approved contents after a malicious version was parsed must not make
// the malicious version pass.
func TestProjectTrustUsesParsedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectFileName)
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	writeTestFile(t, path, "entry = \"warden\"\n")
	if _, err := ApproveProject(TrustPath(), loadTestProject(t, dir)); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}

	writeTestFile(t, path, "entry = \"aws-prod\"\n")
	evil := loadTestProject(t, dir)
	writeTestFile(t, path, "entry = \"warden\"\n") // swap back before the check

	ok, err := IsProjectTrusted(TrustPath(), evil)
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatalf("a project parsed as entry %q passed approval of different bytes", evil.Entry)
	}
}

func TestTrustStoreInsideProjectIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ProjectFileName), "entry = \"aws-prod\"\n")
	t.Setenv("SANDUK_TRUST", filepath.Join(dir, ".trusted.toml"))

	p := loadTestProject(t, dir)
	if _, err := ApproveProject(TrustPath(), p); err == nil {
		t.Fatal("approved into a trust store inside the project")
	}
	if _, err := IsProjectTrusted(TrustPath(), p); err == nil {
		t.Fatal("consulted a trust store inside the project")
	}
}
