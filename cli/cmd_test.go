package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// testEnv points every sanduk setting at temporary locations and the age
// backend, so a command run through runCmd can never touch the real keychain
// or the developer's config. It returns the config path.
func testEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("SANDUK_BACKEND", "file")
	t.Setenv("SANDUK_FILE_DIR", t.TempDir())
	t.Setenv("SANDUK_FILE_PASSPHRASE", "test-passphrase")
	t.Setenv("SANDUK_SERVICE", "sanduk-cli-test")
	t.Setenv("SANDUK_KEYCHAIN", "")
	t.Setenv("SANDUK_TRUST", "")
	cfg := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("SANDUK_CONFIG", cfg)
	return cfg
}

// runCmd runs sanduk with args as a fresh invocation and returns what it wrote
// to stdout. stdin, when non-empty, replaces os.Stdin for the run.
func runCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	return runApp(t, newApp(), stdin, args...)
}

// runApp is runCmd with a caller-supplied app, for tests that inspect it.
func runApp(t *testing.T, a *app, stdin string, args ...string) (string, error) {
	t.Helper()
	if stdin != "" {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(stdin); err != nil {
			t.Fatal(err)
		}
		w.Close()
		old := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = old; r.Close() })
	}
	root := newRootCmd(a)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestBrokenConfigStillHonoursBackendFlagAndRefusesWrites(t *testing.T) {
	cfg := testEnv(t)
	t.Setenv("SANDUK_BACKEND", "")
	if err := writeFileString(cfg, "backend = [broken\n"); err != nil {
		t.Fatal(err)
	}

	// A broken config used to skip resolving the settings, so --backend was
	// dropped and the write went to the platform keystore. Now the flag is
	// honoured and the write is refused before it reaches any backend.
	a := newApp()
	if _, err := runApp(t, a, "sk-test-value", "--backend", "file", "add", "openai"); err == nil {
		t.Fatal("add succeeded with an unreadable config")
	}
	if s := a.settings; s == nil || s.Backend != "file" || s.BackendFrom != sourceFlag {
		t.Fatalf("settings = %+v, want backend file from flag", s)
	}

	// `sanduk config` must still work, to inspect the broken file.
	out, err := runCmd(t, "", "config")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if !strings.Contains(out, "[broken") {
		t.Fatalf("config did not print the file: %q", out)
	}
}

func TestConfigFlagMovesTrustStore(t *testing.T) {
	testEnv(t)
	a := newApp()
	a.configFlag = filepath.Join(t.TempDir(), "elsewhere", "config.toml")

	if got, want := a.trustPath(), filepath.Join(filepath.Dir(a.configFlag), vault.TrustFileName); got != want {
		t.Fatalf("trustPath = %q, want %q (next to --config)", got, want)
	}
}

func TestFlatImportRefusesOverwrite(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	env := filepath.Join(dir, "a.env")
	if err := writeFileString(env, "OPENAI_API_KEY=sk-test-one\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "import", env); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := writeFileString(env, "OPENAI_API_KEY=sk-test-two\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "import", env); err == nil {
		t.Fatal("second import overwrote an existing entry without --replace")
	}
	if _, err := runCmd(t, "", "import", "--replace", env); err != nil {
		t.Fatalf("import --replace: %v", err)
	}

	v, err := newApp().openVault()
	if err != nil {
		t.Fatal(err)
	}
	e, err := v.Get("OPENAI_API_KEY")
	if err != nil || e.Value != "sk-test-two" {
		t.Fatalf("after --replace: %v (err %v)", e, err)
	}
}

func TestImportRejectsInjectedKeyBeforeWriting(t *testing.T) {
	testEnv(t)
	env := filepath.Join(t.TempDir(), "evil.env")
	if err := writeFileString(env, "GOOD=1\nLD_PRELOAD=/tmp/x.so\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "import", env); err == nil {
		t.Fatal("import accepted LD_PRELOAD")
	}
	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("a refused import left entries behind: %q", out)
	}
}

func TestExportOutputRefusesOverwrite(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}
	dst := filepath.Join(t.TempDir(), ".env")
	if _, err := runCmd(t, "", "export", "openai", "-f", "dotenv", "-o", dst); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := runCmd(t, "", "export", "openai", "-f", "dotenv", "-o", dst); err == nil {
		t.Fatal("export -o replaced an existing file without --replace")
	}
	if _, err := runCmd(t, "", "export", "openai", "-f", "dotenv", "-o", dst, "--replace"); err != nil {
		t.Fatalf("export --replace: %v", err)
	}
}

func TestExportWritesToCommandOutput(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, err := runCmd(t, "", "export", "openai")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if out != "export OPENAI_API_KEY='sk-test-value'\n" {
		t.Fatalf("export output = %q", out)
	}
}

func TestGetRefusesNonTerminal(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "openai"); err != nil {
		t.Fatalf("add: %v", err)
	}
	// runCmd's stdout is a buffer, never a terminal.
	if out, err := runCmd(t, "", "get", "openai"); err == nil {
		t.Fatalf("get printed to a non-terminal: %q", out)
	}
	out, err := runCmd(t, "", "get", "openai", "--force")
	if err != nil || out != redact("sk-test-value")+"\n" {
		t.Fatalf("get --force = %q, %v (want redacted)", out, err)
	}
	out, err = runCmd(t, "", "get", "openai", "--force", "--reveal")
	if err != nil || out != "sk-test-value\n" {
		t.Fatalf("get --force --reveal = %q, %v", out, err)
	}
}

func TestRemoveCleansOrphanMetadata(t *testing.T) {
	cfg := testEnv(t)
	if err := writeFileString(cfg, "[secrets.ghost]\nenv = \"GHOST\"\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "remove", "ghost", "--yes"); err != nil {
		t.Fatalf("remove of an orphaned record: %v", err)
	}
	c, err := vault.LoadConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Secrets["ghost"]; ok {
		t.Fatal("orphaned metadata survived remove")
	}
	if _, err := runCmd(t, "", "remove", "never-existed", "--yes"); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("remove of an unknown name = %v, want ErrNotFound", err)
	}
}

func TestDoctorMetadataFindsAndFixesDrift(t *testing.T) {
	cfg := testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "openai"); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Lose openai's metadata and leave a record for an entry that is gone.
	if err := writeFileString(cfg, "[groups.gone]\n"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "doctor", "--metadata")
	var exit *vault.ExitError
	if !errors.As(err, &exit) || exit.Code != exitDrift {
		t.Fatalf("doctor --metadata = %v, want exit %d", err, exitDrift)
	}
	if !strings.Contains(out, "openai") || !strings.Contains(out, "gone") {
		t.Fatalf("drift report = %q", out)
	}
	if strings.Contains(out, "sk-test-value") {
		t.Fatal("drift report printed a secret value")
	}

	if _, err := runCmd(t, "", "doctor", "--metadata", "--fix"); err != nil {
		t.Fatalf("--fix: %v", err)
	}
	if _, err := runCmd(t, "", "doctor", "--metadata"); err != nil {
		t.Fatalf("drift remains after --fix: %v", err)
	}
}

func TestAddGroupFromJSONFile(t *testing.T) {
	testEnv(t)
	src := filepath.Join(t.TempDir(), "f.json")
	if err := writeFileString(src, `{"DB_URL":"postgres://test","API_KEY":"sk-test"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "warden", "--from-json", src); err != nil {
		t.Fatalf("add-group --from-json FILE: %v", err)
	}
	if _, err := runCmd(t, `{"A":"1"}`, "add-group", "other", "--from-json", "-"); err != nil {
		t.Fatalf("add-group --from-json -: %v", err)
	}
	out, err := runCmd(t, "", "group", "show", "-g", "warden")
	if err != nil || !strings.Contains(out, "API_KEY") || !strings.Contains(out, "DB_URL") {
		t.Fatalf("group show = %q, %v", out, err)
	}
}

// add judges an overwrite by what the backend holds, not by metadata that can
// drift: config claiming "secret" must not let add destroy a group.
func TestAddRefusesGroupDespiteDriftedMetadata(t *testing.T) {
	cfg := testEnv(t)
	src := filepath.Join(t.TempDir(), "w.env")
	if err := writeFileString(src, "DB_URL=postgres://test\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "warden", "--from-json", dotenvAsJSON(t, src)); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	if err := writeFileString(cfg, "[secrets.warden]\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sk-test-value", "add", "warden"); err == nil {
		t.Fatal("add overwrote a group because the metadata said secret")
	}
}
