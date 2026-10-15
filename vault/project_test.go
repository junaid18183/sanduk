package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectWalksUp(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ProjectFileName), "entry = \"warden\"\nonly = [\"DB_URL\"]\n")

	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	path, proj, err := LoadProject(deep)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if proj == nil {
		t.Fatal("expected a project file to be found")
	}
	if proj.Entry != "warden" {
		t.Fatalf("entry = %q, want warden", proj.Entry)
	}
	if len(proj.Only) != 1 || proj.Only[0] != "DB_URL" {
		t.Fatalf("only = %v, want [DB_URL]", proj.Only)
	}
	if want := filepath.Join(root, ProjectFileName); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestLoadProjectNearestWins(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ProjectFileName), "entry = \"outer\"\n")

	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(inner, ProjectFileName), "entry = \"inner\"\n")

	_, proj, err := LoadProject(inner)
	if err != nil {
		t.Fatal(err)
	}
	if proj.Entry != "inner" {
		t.Fatalf("entry = %q, want the nearest file to win", proj.Entry)
	}
}

func TestLoadProjectAbsent(t *testing.T) {
	path, proj, err := LoadProject(t.TempDir())
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if path != "" || proj != nil {
		t.Fatalf("expected no project, got (%q, %+v)", path, proj)
	}
}

func TestLoadProjectRequiresEntry(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ProjectFileName), "only = [\"A\"]\n")

	if _, _, err := LoadProject(dir); err == nil {
		t.Fatal("expected an error for a project file without an entry")
	}
}
