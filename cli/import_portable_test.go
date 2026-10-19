package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := writeFileString(path, content); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportCSVCreatesSecrets(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, "export.csv", "name,login_password\nStripe,sk-test-one\nOpenAI,sk-test-two\n")

	if _, err := runCmd(t, "", "import", path); err != nil {
		t.Fatalf("import: %v", err)
	}

	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"STRIPE", "OPENAI"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list = %q, missing %s", out, want)
		}
	}
	// The value made it in intact.
	got, err := runCmd(t, "", "export", "STRIPE", "--format", "dotenv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "STRIPE=sk-test-one") {
		t.Fatalf("export = %q", got)
	}
}

func TestImportDryRunPrintsNamesNotValues(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, "export.csv", "name,login_password\nStripe,sk-test-secret\n")

	out, err := runCmd(t, "", "import", path, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "STRIPE") {
		t.Fatalf("out = %q", out)
	}
	if strings.Contains(out, "sk-test-secret") {
		t.Fatalf("dry run printed a value: %q", out)
	}
	// Nothing was written.
	names, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(names, "STRIPE") {
		t.Fatalf("dry run wrote an entry: %q", names)
	}
}

func TestImportCSVIntoAGroup(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, "export.csv", "name,login_password\nStripe,sk-test-one\nOpenAI,sk-test-two\n")

	if _, err := runCmd(t, "", "import", path, "--group", "imported"); err != nil {
		t.Fatalf("import: %v", err)
	}
	out, err := runCmd(t, "", "group", "show", "imported")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "STRIPE") || !strings.Contains(out, "OPENAI") {
		t.Fatalf("group show = %q", out)
	}
}

func TestImportFormatIsForcible(t *testing.T) {
	testEnv(t)
	// A CSV that does not end in .csv would otherwise be read as dotenv.
	path := writeTemp(t, "export.txt", "name,login_password\nStripe,sk-test-one\n")

	if _, err := runCmd(t, "", "import", path, "--format", "csv"); err != nil {
		t.Fatalf("import --format csv: %v", err)
	}
	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "STRIPE") {
		t.Fatalf("list = %q", out)
	}
}

func TestImportRejectsAnUnknownFormat(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, "export.csv", "name,login_password\nStripe,sk-test-one\n")
	if _, err := runCmd(t, "", "import", path, "--format", "keepass"); err == nil {
		t.Fatal("an unknown format was accepted")
	}
}

// A name that collides with the loader deny-list must fail the whole import,
// not store some entries and then stop.
func TestImportRefusesADeniedNameBeforeWriting(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, "export.csv", "name,login_password\nStripe,sk-test-one\nLD_PRELOAD,sk-test-evil\n")

	if _, err := runCmd(t, "", "import", path); err == nil {
		t.Fatal("a denied variable name was imported")
	}
	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "STRIPE") {
		t.Fatalf("import was left half done: %q", out)
	}
}

func TestImportDotenvStillWorks(t *testing.T) {
	testEnv(t)
	path := writeTemp(t, ".env.production", "DB_URL=postgres://x\nAPI_KEY=sk-test-one\n")

	if _, err := runCmd(t, "", "import", path); err != nil {
		t.Fatalf("import: %v", err)
	}
	out, err := runCmd(t, "", "list", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "DB_URL") || !strings.Contains(out, "API_KEY") {
		t.Fatalf("list = %q", out)
	}
}
