package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

func TestScanFindsALeakedValue(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-abcdefghij", "add", "stripe"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, "config.py"), "KEY = 'sk-test-abcdefghij'\n"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "scan", dir, "--yes")
	if !strings.Contains(out, "stripe") || !strings.Contains(out, "config.py") {
		t.Fatalf("scan = %q", out)
	}
	// The report must not contain the secret itself.
	if strings.Contains(out, "sk-test-abcdefghij") {
		t.Fatalf("scan printed the value: %q", out)
	}
	var ee *vault.ExitError
	if !errors.As(err, &ee) || ee.Code != exitDrift {
		t.Fatalf("err = %v, want exit %d", err, exitDrift)
	}
}

func TestScanCleanTreeExitsZero(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-abcdefghij", "add", "stripe"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, "config.py"), "KEY = os.environ['STRIPE']\n"); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, "", "scan", dir, "--yes"); err != nil {
		t.Fatalf("clean tree: %v", err)
	}
}

// Reading every value is the one thing scan does that nothing else does, so
// it must not happen silently in a pipe or an agent session.
func TestScanRequiresConsentWithoutATerminal(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-abcdefghij", "add", "stripe"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "", "scan", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("err = %v, want a refusal pointing at --yes", err)
	}
}

func TestScanFindsAGroupField(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, `{"DB_URL":"postgres://user:sk-test-pass@host"}`,
		"add-group", "warden", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, ".env"),
		"DB_URL=postgres://user:sk-test-pass@host\n"); err != nil {
		t.Fatal(err)
	}

	out, _ := runCmd(t, "", "scan", dir, "--yes")
	if !strings.Contains(out, "warden.DB_URL") {
		t.Fatalf("scan = %q, want the group field named", out)
	}
}

func TestScanFindsALeakedFileSecret(t *testing.T) {
	testEnv(t)
	src := writeTemp(t, "sa.json", `{"private_key":"sk-test-service-account"}`)
	if _, err := runCmd(t, "", "add-file", "gcp", "--from-file", src); err != nil {
		t.Fatal(err)
	}
	// The same file committed into a project.
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, "creds.json"),
		`{"private_key":"sk-test-service-account"}`); err != nil {
		t.Fatal(err)
	}

	out, _ := runCmd(t, "", "scan", dir, "--yes")
	if !strings.Contains(out, "gcp") {
		t.Fatalf("scan = %q, want the file secret named", out)
	}
}

func TestScanSkipsVendorDirectories(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-abcdefghij", "add", "stripe"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	nested := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileString(filepath.Join(nested, "index.js"), "const k='sk-test-abcdefghij'\n"); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, "", "scan", dir, "--yes"); err != nil {
		t.Fatalf("node_modules should not be scanned: %v", err)
	}
}

func TestScanWarnsAboutShortValues(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "abc", "add", "tiny"); err != nil {
		t.Fatal(err)
	}
	// Nothing searchable: the command still succeeds rather than erroring.
	if _, err := runCmd(t, "", "scan", t.TempDir(), "--yes"); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestScanGitHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	testEnv(t)
	if _, err := runCmd(t, "sk-test-abcdefghij", "add", "stripe"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q")
	if err := writeFileString(filepath.Join(dir, "leak.txt"), "sk-test-abcdefghij\n"); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "oops")
	// Delete it: the working tree is clean, but history is not.
	if err := os.Remove(filepath.Join(dir, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "remove it")

	// The working tree alone looks fine.
	if _, err := runCmd(t, "", "scan", dir, "--yes"); err != nil {
		t.Fatalf("working tree should be clean: %v", err)
	}
	// History still holds the secret.
	out, err := runCmd(t, "", "scan", dir, "--git", "--yes")
	if !strings.Contains(out, "git history") {
		t.Fatalf("scan --git = %q", out)
	}
	var ee *vault.ExitError
	if !errors.As(err, &ee) || ee.Code != exitDrift {
		t.Fatalf("err = %v, want exit %d", err, exitDrift)
	}
}
