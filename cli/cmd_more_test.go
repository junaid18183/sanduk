package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// projectDir creates a repository-like directory with a .sanduk.toml and an
// .env.example, and makes it the working directory.
func projectDir(t *testing.T, project, template string) string {
	t.Helper()
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, vault.ProjectFileName), project); err != nil {
		t.Fatal(err)
	}
	if template != "" {
		if err := writeFileString(filepath.Join(dir, defaultTemplateName), template); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func addWarden(t *testing.T) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "warden.env")
	if err := writeFileString(src, "DB_URL=postgres://test\nAPI_KEY=sk-test-api\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "warden", "--from-dotenv", src, "--description", "test group"); err != nil {
		t.Fatalf("add-group: %v", err)
	}
}

func TestAllowThenExecListUsesProject(t *testing.T) {
	testEnv(t)
	addWarden(t)
	projectDir(t, "entry = \"warden\"\nonly = [\"DB_URL\"]\n", "")

	// Unapproved: exec, export and doctor all refuse.
	for _, args := range [][]string{
		{"exec", "--list", "--", "true"},
		{"export"},
		{"doctor"},
	} {
		if _, err := runCmd(t, "", args...); err == nil || !strings.Contains(err.Error(), "not approved") {
			t.Fatalf("%v before allow = %v, want a not-approved error", args, err)
		}
	}

	if _, err := runCmd(t, "", "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	out, err := runCmd(t, "", "exec", "--list", "--", "true")
	if err != nil {
		t.Fatalf("exec --list: %v", err)
	}
	// The project's `only` narrows the group to DB_URL; names only, no values.
	if out != "DB_URL\n" {
		t.Fatalf("exec --list = %q, want DB_URL only", out)
	}
	if strings.Contains(out, "postgres") {
		t.Fatal("exec --list printed a value")
	}

	// --only on the command line replaces the project's subset.
	out, err = runCmd(t, "", "exec", "--list", "--only", "API_KEY", "--", "true")
	if err != nil || out != "API_KEY\n" {
		t.Fatalf("exec --list --only API_KEY = %q, %v", out, err)
	}
}

func TestAllowWithoutProjectFile(t *testing.T) {
	testEnv(t)
	t.Chdir(t.TempDir())
	if _, err := runCmd(t, "", "allow"); err == nil {
		t.Fatal("allow succeeded with no project file")
	}
}

func TestDoctorReportsMissingAndExtra(t *testing.T) {
	testEnv(t)
	addWarden(t)
	projectDir(t, "entry = \"warden\"\n", "DB_URL=x\nSTRIPE_KEY=x\n")
	if _, err := runCmd(t, "", "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}

	out, err := runCmd(t, "", "doctor")
	var exit *vault.ExitError
	if !errors.As(err, &exit) || exit.Code != exitDrift {
		t.Fatalf("doctor = %v, want exit %d", err, exitDrift)
	}
	for _, want := range []string{"DB_URL", "ok", "STRIPE_KEY", "missing", "API_KEY", "extra"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-test-api") {
		t.Fatal("doctor printed a value")
	}

	// An explicit entry and template need no project file.
	tmpl := filepath.Join(t.TempDir(), "t.env")
	if err := writeFileString(tmpl, "DB_URL=x\nAPI_KEY=x\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "doctor", "warden", "--template", tmpl); err != nil {
		t.Fatalf("doctor with a matching template: %v", err)
	}
	if _, err := runCmd(t, "", "doctor", "warden", "--fix"); err == nil {
		t.Fatal("--fix without --metadata was accepted")
	}
}

func TestGroupSetUnsetShowGet(t *testing.T) {
	testEnv(t)
	addWarden(t)

	// group set KEY reads a single value from piped stdin.
	if _, err := runCmd(t, "sk-test-stripe", "group", "set", "warden", "STRIPE_KEY"); err != nil {
		t.Fatalf("group set: %v", err)
	}
	out, err := runCmd(t, "", "group", "show", "warden")
	if err != nil {
		t.Fatalf("group show: %v", err)
	}
	if out != "API_KEY\nDB_URL\nSTRIPE_KEY\n" {
		t.Fatalf("group show = %q", out)
	}

	out, err = runCmd(t, "", "group", "get", "warden", "STRIPE_KEY", "--force")
	if err != nil || out != redact("sk-test-stripe")+"\n" {
		t.Fatalf("group get (redacted) = %q, %v", out, err)
	}
	out, err = runCmd(t, "", "group", "get", "warden", "STRIPE_KEY", "--force", "--reveal")
	if err != nil || out != "sk-test-stripe\n" {
		t.Fatalf("group get --reveal = %q, %v", out, err)
	}
	if _, err := runCmd(t, "", "group", "get", "warden", "NOPE", "--force"); err == nil {
		t.Fatal("group get of an unknown field succeeded")
	}

	if _, err := runCmd(t, "", "group", "unset", "warden", "STRIPE_KEY"); err != nil {
		t.Fatalf("group unset: %v", err)
	}
	if _, err := runCmd(t, "", "group", "unset", "warden", "API_KEY", "DB_URL"); err == nil {
		t.Fatal("group unset removed every field")
	}

	// --from-env reads each key from the variable of the same name.
	t.Setenv("REDIS_URL", "redis://test")
	if _, err := runCmd(t, "", "group", "set", "warden", "REDIS_URL", "--from-env"); err != nil {
		t.Fatalf("group set --from-env: %v", err)
	}
	if _, err := runCmd(t, "", "group", "set", "warden", "LD_PRELOAD", "--from-env"); err == nil {
		t.Fatal("group set accepted LD_PRELOAD")
	}
}

func TestGetGroupFieldAndAll(t *testing.T) {
	testEnv(t)
	addWarden(t)
	out, err := runCmd(t, "", "get", "warden", "--field", "API_KEY", "--force")
	if err != nil || out != redact("sk-test-api")+"\n" {
		t.Fatalf("get --field = %q, %v", out, err)
	}
	// --all is redacted like everything else get prints, unless --reveal.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"get", "warden", "--all", "--force"}, redact("postgres://test")},
		{[]string{"get", "warden", "--all", "--force", "--reveal"}, "postgres://test"},
	} {
		out, err = runCmd(t, "", c.args...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(out), &m); err != nil || m["DB_URL"] != c.want {
			t.Fatalf("%v = %q (%v), want DB_URL %q", c.args, out, err, c.want)
		}
	}
	if _, err := runCmd(t, "", "get", "warden", "--force"); err == nil {
		t.Fatal("get of a group without --field/--all succeeded")
	}
}

func TestRotateReplacesValue(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-old", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := runCmd(t, "sk-test-new", "rotate", "openai"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	out, err := runCmd(t, "", "export", "openai", "-f", "json")
	if err != nil || !strings.Contains(out, "sk-test-new") {
		t.Fatalf("after rotate export = %q, %v", out, err)
	}
	if _, err := runCmd(t, "sk-test-x", "rotate", "missing"); err == nil {
		t.Fatal("rotate of a missing entry succeeded")
	}
}

func TestListFormats(t *testing.T) {
	testEnv(t)
	addWarden(t)
	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY", "--description", "OpenAI"); err != nil {
		t.Fatalf("add: %v", err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"NAME", "openai", "secret", "OPENAI_API_KEY", "warden", "group"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-test") || strings.Contains(out, "postgres") {
		t.Fatalf("list printed a value:\n%s", out)
	}

	out, err = runCmd(t, "", "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var doc struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Entries) != 2 {
		t.Fatalf("list --json = %q (%v)", out, err)
	}
	// Every row says which scope it is in, so a caller can tell a project's
	// entries from the ones that stand on their own.
	for _, r := range doc.Entries {
		if r["scope"] != "global" {
			t.Fatalf("row %v: want scope global outside any project", r)
		}
	}
}

func TestAddReplaceRules(t *testing.T) {
	testEnv(t)
	addWarden(t)
	if _, err := runCmd(t, "sk-test-value", "add", "warden"); err == nil {
		t.Fatal("add replaced a group without --replace")
	}
	if _, err := runCmd(t, "", "add-group", "warden", "--from-json", "-"); err == nil {
		t.Fatal("add-group replaced an existing group without --replace")
	}
	if _, err := runCmd(t, "sk-test-value", "add", "warden", "--replace", "--as", "WARDEN_TOKEN"); err != nil {
		t.Fatalf("add --replace: %v", err)
	}
	out, err := runCmd(t, "", "list", "--json")
	if err != nil || !strings.Contains(out, `"secret"`) || strings.Contains(out, `"group"`) {
		t.Fatalf("after replace list = %q, %v", out, err)
	}
}

func TestCompletionListsEntries(t *testing.T) {
	testEnv(t)
	addWarden(t)
	names, _ := newApp().completeEntryNames(nil, nil, "")
	if len(names) != 1 || names[0] != "warden" {
		t.Fatalf("completion = %v, want [warden]", names)
	}
	// With the age backend and no passphrase available, completion must not
	// prompt (it would corrupt the shell's completion output).
	t.Setenv("SANDUK_FILE_PASSPHRASE", "")
	if names, _ := newApp().completeSelector(nil, nil, ""); names != nil {
		t.Fatalf("completion read the vault without a passphrase: %v", names)
	}
}

func TestVersionCommand(t *testing.T) {
	testEnv(t)
	out, err := runCmd(t, "", "version")
	if err != nil || !strings.HasPrefix(out, "sanduk ") {
		t.Fatalf("version = %q, %v", out, err)
	}
}
