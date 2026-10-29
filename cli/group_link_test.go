package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// seedLinkable creates the shape the feature exists for: one flat secret shared
// by a project group.
func seedLinkable(t *testing.T) {
	t.Helper()
	testEnv(t)
	if _, err := runCmd(t, "sk-test-one", "add", "elevenlabs", "--as", "ELEVENLABS_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "maktub", "--from-json", "-"); err == nil {
		t.Fatal("expected the empty stdin JSON to fail")
	}
	if _, err := runCmd(t, `{"MAKTUB_DB_URL":"postgres://x"}`, "add-group", "maktub", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
}

func TestGroupLinkUsesTheSecretsConfiguredEnvName(t *testing.T) {
	seedLinkable(t)

	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatalf("link: %v", err)
	}

	out, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(out, "ELEVENLABS_API_KEY=sk-test-one") {
		t.Fatalf("export = %q", out)
	}
}

func TestGroupLinkBindsAnExplicitName(t *testing.T) {
	seedLinkable(t)

	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "XI_KEY=elevenlabs"); err != nil {
		t.Fatalf("link: %v", err)
	}

	out, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "XI_KEY=sk-test-one") {
		t.Fatalf("export = %q", out)
	}
	if strings.Contains(out, "ELEVENLABS_API_KEY") {
		t.Fatalf("explicit binding should not also use the default name: %q", out)
	}
}

// The single-source-of-truth guarantee, end to end through the CLI.
func TestRotatingTheSecretUpdatesTheGroup(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, "sk-test-rotated", "rotate", "elevenlabs"); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	out, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ELEVENLABS_API_KEY=sk-test-rotated") {
		t.Fatalf("group did not follow the rotation: %q", out)
	}
}

func TestGroupShowListsLinksPlainAndLong(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	// Default output stays one bare name per line, so it keeps piping into
	// --only and xargs.
	out, err := runCmd(t, "", "group", "show", "-g", "maktub")
	if err != nil {
		t.Fatal(err)
	}
	want := "ELEVENLABS_API_KEY\nMAKTUB_DB_URL\n"
	if out != want {
		t.Fatalf("show = %q, want %q", out, want)
	}

	out, err = runCmd(t, "", "group", "show", "-g", "maktub", "--long")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "-> elevenlabs") || !strings.Contains(out, "stored") {
		t.Fatalf("show --long = %q", out)
	}
}

func TestGroupUnlinkKeepsTheSecret(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, "", "group", "unlink", "-g", "maktub", "ELEVENLABS_API_KEY"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if _, err := runCmd(t, "", "export", "elevenlabs", "--format", "dotenv"); err != nil {
		t.Fatalf("unlink removed the secret: %v", err)
	}
	out, err := runCmd(t, "", "group", "show", "-g", "maktub")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ELEVENLABS_API_KEY") {
		t.Fatalf("show still lists the link: %q", out)
	}
}

func TestExecRefusesADanglingLink(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "remove", "elevenlabs", "--yes"); err != nil {
		t.Fatal(err)
	}

	_, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err == nil {
		t.Fatal("export succeeded with a dangling link")
	}
	if !strings.Contains(err.Error(), "elevenlabs") {
		t.Fatalf("error does not name the target: %v", err)
	}
}

func TestDoctorLinksReportsDrift(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "doctor", "--links")
	if err != nil {
		t.Fatalf("doctor --links: %v (%s)", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("doctor = %q", out)
	}

	if _, err := runCmd(t, "", "remove", "elevenlabs", "--yes"); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, "", "doctor", "--links")
	if !strings.Contains(out, "missing") {
		t.Fatalf("doctor = %q, want a missing row", out)
	}
	var ee *vault.ExitError
	if !errors.As(err, &ee) || ee.Code != exitDrift {
		t.Fatalf("err = %v, want exit %d", err, exitDrift)
	}
}

// A linked field satisfies a template key: doctor must not call it missing.
func TestDoctorTemplateCountsLinkedFields(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	tmpl := filepath.Join(t.TempDir(), ".env.example")
	if err := writeFileString(tmpl, "ELEVENLABS_API_KEY=\nMAKTUB_DB_URL=\n"); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "doctor", "maktub", "--template", tmpl)
	if err != nil {
		t.Fatalf("doctor: %v (%s)", err, out)
	}
	if strings.Contains(out, "missing") {
		t.Fatalf("doctor reported a link as missing: %q", out)
	}
}

func TestGroupLinkRefusesAGroupTarget(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, `{"A":"b"}`, "add-group", "warden", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}

	_, err := runCmd(t, "", "group", "link", "-g", "maktub", "warden")
	if err == nil || !strings.Contains(err.Error(), "flat secret") {
		t.Fatalf("err = %v, want a flat-secret refusal", err)
	}
}

func TestGroupLinkRefusesAStoredField(t *testing.T) {
	seedLinkable(t)
	_, err := runCmd(t, "", "group", "link", "-g", "maktub", "MAKTUB_DB_URL=elevenlabs")
	if err == nil || !strings.Contains(err.Error(), "already stores") {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

// Only add-group creates a group: a link into a missing one is refused, so a
// mistyped -g cannot start a new group.
func TestLinkRefusesAMissingGroup(t *testing.T) {
	seedLinkable(t)
	_, err := runCmd(t, "", "group", "link", "-g", "fresh", "elevenlabs")
	if err == nil || !strings.Contains(err.Error(), "add-group") {
		t.Fatalf("link into a missing group: err = %v", err)
	}
}

func TestLinkedGroupIsRecordedAsAGroup(t *testing.T) {
	seedLinkable(t)
	if _, err := runCmd(t, `{"OTHER":"x"}`, "add-group", "fresh", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "group", "link", "-g", "fresh", "elevenlabs"); err != nil {
		t.Fatalf("link: %v", err)
	}
	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fresh") || !strings.Contains(out, "group") {
		t.Fatalf("list = %q", out)
	}
}
