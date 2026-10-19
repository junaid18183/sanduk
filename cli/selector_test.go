package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

func writeFileString(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// newTestVault opens a vault against the encrypted file backend and a temporary
// metadata config, so tests never touch the real keychain or the real config.
func newTestVault(t *testing.T) *vault.Vault {
	t.Helper()
	t.Setenv("SANDUK_BACKEND", "file")
	t.Setenv("SANDUK_FILE_DIR", t.TempDir())
	t.Setenv("SANDUK_FILE_PASSPHRASE", "test-passphrase")
	t.Setenv("SANDUK_SERVICE", "sanduk-cli-test")
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	v, err := newApp().openVault()
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	return v
}

func TestParseSelector(t *testing.T) {
	cases := []struct {
		in       string
		wantEnv  string
		wantName string
	}{
		{"openai", "", "openai"},
		{"OPENAI_API_KEY=openai", "OPENAI_API_KEY", "openai"},
		{"warden", "", "warden"},
		// A "=" only binds when both sides are non-empty, so these stay names.
		{"=openai", "", "=openai"},
		{"openai=", "", "openai="},
		{"=", "", "="},
	}

	for _, c := range cases {
		got := parseSelector(c.in)
		if got.env != c.wantEnv || got.name != c.wantName {
			t.Errorf("parseSelector(%q) = {%q, %q}, want {%q, %q}",
				c.in, got.env, got.name, c.wantEnv, c.wantName)
		}
	}
}

func TestSplitOnly(t *testing.T) {
	got := splitOnly([]string{"A,B", " C ", "", "D"})
	want := []string{"A", "B", "C", "D"}
	if len(got) != len(want) {
		t.Fatalf("splitOnly = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitOnly = %v, want %v", got, want)
		}
	}

	if got := splitOnly(nil); got != nil {
		t.Fatalf("splitOnly(nil) = %v, want nil", got)
	}
}

func TestFilterOnly(t *testing.T) {
	sel := selection{env: map[string]string{"A": "1", "B": "2", "C": "3"}}

	// No allowlist leaves the environment untouched.
	got, err := filterOnly(sel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.env) != 3 {
		t.Fatalf("filtered = %v, want all keys", got.env)
	}

	got, err = filterOnly(sel, []string{"A,C"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.env) != 2 || got.env["A"] != "1" || got.env["C"] != "3" {
		t.Fatalf("filtered = %v, want A and C", got.env)
	}

	// A typo must fail rather than silently injecting nothing.
	if _, err := filterOnly(sel, []string{"A,NOPE"}); err == nil {
		t.Fatal("expected an error for an unknown key")
	}
}

func TestResolveSelectorsMergesLeftToRight(t *testing.T) {
	v := newTestVault(t)
	cfg := &vault.Config{}

	if err := v.AddGroup("warden", map[string]string{"DB_URL": "group-value", "API_KEY": "k"}); err != nil {
		t.Fatal(err)
	}
	if err := v.AddSecret("db-alt", "flat-value"); err != nil {
		t.Fatal(err)
	}

	sel, err := resolveSelectors(v, cfg, parseSelectors([]string{"warden", "DB_URL=db-alt"}))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	env := sel.env
	if env["DB_URL"] != "flat-value" {
		t.Fatalf("DB_URL = %q, want the later selector to win", env["DB_URL"])
	}
	if env["API_KEY"] != "k" {
		t.Fatalf("API_KEY = %q, want the group's other field", env["API_KEY"])
	}

	sel, err = resolveSelectors(v, cfg, parseSelectors([]string{"DB_URL=db-alt", "warden"}))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	env = sel.env
	if env["DB_URL"] != "group-value" {
		t.Fatalf("DB_URL = %q, want the group to win when it comes last", env["DB_URL"])
	}
}

func TestResolveSelectorsRejectsGroupBinding(t *testing.T) {
	v := newTestVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveSelectors(v, &vault.Config{}, parseSelectors([]string{"A=warden"})); err == nil {
		t.Fatal("expected binding a group to a single variable to fail")
	}
}

func TestResolveSelectorsMissingEntry(t *testing.T) {
	v := newTestVault(t)
	if _, err := resolveSelectors(v, &vault.Config{}, parseSelectors([]string{"nope"})); err == nil {
		t.Fatal("expected an error for a missing entry")
	}
}

func TestDefaultEntryFailsWithoutProjectFile(t *testing.T) {
	// A temporary directory has no .sanduk.toml in its ancestry.
	t.Chdir(t.TempDir())
	if _, _, err := newApp().defaultEntry(); err == nil {
		t.Fatal("expected an error when no project file exists")
	}
}

func TestDefaultEntryFromProjectFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, vault.ProjectFileName)
	if err := writeFileString(path, "entry = \"warden\"\nonly = [\"DB_URL\"]\n"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	// A project file must be approved before it is used; approve it as
	// `sanduk allow` would, writing the trust store to a temporary config dir.
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	_, proj, err := vault.LoadProject(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := vault.ApproveProject(newApp().trustPath(), proj); err != nil {
		t.Fatalf("approve: %v", err)
	}

	entry, only, err := newApp().defaultEntry()
	if err != nil {
		t.Fatalf("defaultEntry: %v", err)
	}
	if entry != "warden" {
		t.Fatalf("entry = %q, want warden", entry)
	}
	if len(only) != 1 || only[0] != "DB_URL" {
		t.Fatalf("only = %v, want [DB_URL]", only)
	}
}

func TestDefaultEntryRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, vault.ProjectFileName), "entry = \"warden\"\n"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	if _, _, err := newApp().defaultEntry(); err == nil {
		t.Fatal("expected an unapproved project file to be refused")
	}
}

func TestDefaultEntryApprovalRevokedByEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, vault.ProjectFileName)
	if err := writeFileString(path, "entry = \"warden\"\n"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("SANDUK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	_, proj, err := vault.LoadProject(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := vault.ApproveProject(newApp().trustPath(), proj); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Editing the file must revoke the approval.
	if err := writeFileString(path, "entry = \"aws-prod\"\n"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newApp().defaultEntry(); err == nil {
		t.Fatal("expected an edited project file to require re-approval")
	}
}
