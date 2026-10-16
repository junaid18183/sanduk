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
	tp := filepath.Join(t.TempDir(), TrustFileName)

	p := loadTestProject(t, dir)
	ok, err := IsProjectTrusted(tp, p)
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("an unapproved file must not be trusted")
	}

	if _, err := ApproveProject(tp, p); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}
	ok, err = IsProjectTrusted(tp, loadTestProject(t, dir))
	if err != nil || !ok {
		t.Fatalf("approved file should be trusted (ok=%v err=%v)", ok, err)
	}

	// Editing the file must revoke the approval.
	writeTestFile(t, proj, "entry = \"aws-prod\"\n")
	ok, err = IsProjectTrusted(tp, loadTestProject(t, dir))
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("editing the project file must revoke approval")
	}

	store, err := loadTrust(tp)
	if err != nil {
		t.Fatalf("loadTrust: %v", err)
	}
	if len(store.Files) != 1 || store.Files[p.path] == "" {
		t.Fatalf("trust store = %v, want one record for %s", store.Files, p.path)
	}
}

// The trust check must judge the bytes that were parsed. Swapping the file back
// to the approved contents after a malicious version was parsed must not make
// the malicious version pass.
func TestProjectTrustUsesParsedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectFileName)
	tp := filepath.Join(t.TempDir(), TrustFileName)

	writeTestFile(t, path, "entry = \"warden\"\n")
	if _, err := ApproveProject(tp, loadTestProject(t, dir)); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}

	writeTestFile(t, path, "entry = \"aws-prod\"\n")
	evil := loadTestProject(t, dir)
	writeTestFile(t, path, "entry = \"warden\"\n") // swap back before the check

	ok, err := IsProjectTrusted(tp, evil)
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
	tp := filepath.Join(dir, ".trusted.toml")

	p := loadTestProject(t, dir)
	if _, err := ApproveProject(tp, p); err == nil {
		t.Fatal("approved into a trust store inside the project")
	}
	if _, err := IsProjectTrusted(tp, p); err == nil {
		t.Fatal("consulted a trust store inside the project")
	}
}
