package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedFileSecret(t *testing.T) string {
	t.Helper()
	testEnv(t)
	src := writeTemp(t, "sa.json", `{"type":"service_account","private_key":"sk-test-value"}`)
	if _, err := runCmd(t, "", "add-file", "gcp", "--from-file", src,
		"--as", "GOOGLE_APPLICATION_CREDENTIALS"); err != nil {
		t.Fatalf("add-file: %v", err)
	}
	return src
}

func TestAddFileListsAsAFile(t *testing.T) {
	seedFileSecret(t)

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "file") || !strings.Contains(out, "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Fatalf("list = %q", out)
	}
}

func TestAddFileDefaultsTheVariableName(t *testing.T) {
	testEnv(t)
	src := writeTemp(t, "key.p8", "-----BEGIN PRIVATE KEY-----")
	if _, err := runCmd(t, "", "add-file", "apple-key", "--from-file", src); err != nil {
		t.Fatalf("add-file: %v", err)
	}
	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "APPLE_KEY") {
		t.Fatalf("list = %q, want the slugified variable name", out)
	}
}

func TestAddFileFromStdin(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add-file", "cert", "--from-file", "-"); err != nil {
		t.Fatalf("add-file -: %v", err)
	}
	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cert") {
		t.Fatalf("list = %q", out)
	}
}

func TestAddFileRefusesEmptyAndMissing(t *testing.T) {
	testEnv(t)
	empty := writeTemp(t, "empty.json", "")
	if _, err := runCmd(t, "", "add-file", "gcp", "--from-file", empty); err == nil {
		t.Fatal("an empty file was stored")
	}
	if _, err := runCmd(t, "", "add-file", "gcp", "--from-file", filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing file was accepted")
	}
	if _, err := runCmd(t, "", "add-file", "gcp"); err == nil {
		t.Fatal("--from-file should be required")
	}
}

// get is the terminal-facing command: it must describe a file secret, never
// print its bytes.
func TestGetDescribesAFileWithoutPrintingIt(t *testing.T) {
	seedFileSecret(t)

	out, err := runCmd(t, "", "get", "gcp", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "file secret") || !strings.Contains(out, "sa.json") {
		t.Fatalf("get = %q", out)
	}
	if strings.Contains(out, "sk-test-value") {
		t.Fatalf("get printed the file contents: %q", out)
	}

	// Even --reveal must not dump the bytes.
	out, err = runCmd(t, "", "get", "gcp", "--force", "--reveal")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sk-test-value") {
		t.Fatalf("get --reveal dumped the file: %q", out)
	}
}

// export has no value to print for a file secret, so it must refuse rather
// than emit something misleading.
func TestExportRefusesAFileWithoutMaterialize(t *testing.T) {
	seedFileSecret(t)

	_, err := runCmd(t, "", "export", "gcp", "--format", "dotenv")
	if err == nil {
		t.Fatal("export printed a file secret")
	}
	if !strings.Contains(err.Error(), "--materialize") {
		t.Fatalf("err = %v, want a pointer to --materialize", err)
	}
}

func TestExportMaterializeWritesTheFile(t *testing.T) {
	seedFileSecret(t)
	dir := filepath.Join(t.TempDir(), "creds")

	// JSON, not dotenv: a Windows path contains backslashes, which dotenv
	// quotes, and this test wants the path rather than its encoding.
	out, err := runCmd(t, "", "export", "gcp", "--format", "json", "--materialize", dir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("export = %q: %v", out, err)
	}
	path, ok := env["GOOGLE_APPLICATION_CREDENTIALS"]
	if !ok {
		t.Fatalf("export = %q", out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.Contains(string(data), "sk-test-value") {
		t.Fatalf("materialized file = %q", data)
	}
}

func TestMaterializeWithoutFilesIsAnError(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "export", "plain", "--materialize", t.TempDir()); err == nil {
		t.Fatal("--materialize was accepted for a selection with no file secrets")
	}
}

// exec --list must name the variable a file secret will occupy.
func TestExecListNamesTheFileVariable(t *testing.T) {
	seedFileSecret(t)

	out, err := runCmd(t, "", "exec", "gcp", "--list", "--", "true")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "GOOGLE_APPLICATION_CREDENTIALS" {
		t.Fatalf("exec --list = %q", out)
	}
}

// --only and --except name variables, and must work on file secrets too.
func TestOnlyAndExceptCoverFileSecrets(t *testing.T) {
	seedFileSecret(t)
	if _, err := runCmd(t, "sk-test-other", "add", "plain", "--as", "PLAIN_KEY"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "exec", "gcp", "plain", "--only", "GOOGLE_APPLICATION_CREDENTIALS",
		"--list", "--", "true")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "GOOGLE_APPLICATION_CREDENTIALS" {
		t.Fatalf("--only = %q", out)
	}

	out, err = runCmd(t, "", "exec", "gcp", "plain", "--except", "GOOGLE_APPLICATION_CREDENTIALS",
		"--list", "--", "true")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "PLAIN_KEY" {
		t.Fatalf("--except = %q", out)
	}
}

// A file secret can be bound to an ad-hoc variable name like a flat one.
func TestFileSecretAdHocBinding(t *testing.T) {
	seedFileSecret(t)

	out, err := runCmd(t, "", "exec", "CREDS=gcp", "--list", "--", "true")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "CREDS" {
		t.Fatalf("binding = %q", out)
	}
}

// Links point at flat secrets only; a file secret is not one.
func TestGroupLinkRefusesAFileSecret(t *testing.T) {
	seedFileSecret(t)
	if _, err := runCmd(t, `{"A_KEY":"v"}`, "add-group", "warden", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "", "group", "link", "-g", "warden", "gcp")
	if err == nil || !strings.Contains(err.Error(), "flat secret") {
		t.Fatalf("err = %v, want a flat-secret refusal", err)
	}
}

func TestDoctorMetadataFixRecordsFileKind(t *testing.T) {
	seedFileSecret(t)
	// Drop the metadata, then let doctor rediscover it.
	cfgPath := os.Getenv("SANDUK_CONFIG")
	if err := os.WriteFile(cfgPath, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "doctor", "--metadata", "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}
	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "file") {
		t.Fatalf("list = %q, want the file kind restored", out)
	}
}
