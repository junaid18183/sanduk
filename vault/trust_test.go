package vault

import (
	"path/filepath"
	"testing"
)

func TestProjectTrustLifecycle(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, ProjectFileName)
	writeTestFile(t, proj, "entry = \"warden\"\n")
	// Keep the trust store out of the developer's real config directory.
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	ok, err := IsProjectTrusted(proj)
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("an unapproved file must not be trusted")
	}

	if _, err := ApproveProject(proj); err != nil {
		t.Fatalf("ApproveProject: %v", err)
	}
	ok, err = IsProjectTrusted(proj)
	if err != nil || !ok {
		t.Fatalf("approved file should be trusted (ok=%v err=%v)", ok, err)
	}

	// Editing the file must revoke the approval.
	writeTestFile(t, proj, "entry = \"aws-prod\"\n")
	ok, err = IsProjectTrusted(proj)
	if err != nil {
		t.Fatalf("IsProjectTrusted: %v", err)
	}
	if ok {
		t.Fatal("editing the project file must revoke approval")
	}

	paths, err := TrustedPaths()
	if err != nil {
		t.Fatalf("TrustedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != proj {
		t.Fatalf("TrustedPaths = %v, want [%s]", paths, proj)
	}
}
