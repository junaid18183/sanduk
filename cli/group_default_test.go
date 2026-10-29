package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Without -g, a group command edits the entry of the approved .sanduk.toml here,
// the same default exec and export use.
func TestGroupCommandsDefaultToTheProjectEntry(t *testing.T) {
	testEnv(t)
	addWarden(t)
	projectDir(t, "entry = \"warden\"\n", "")

	if _, err := runCmd(t, "", "group", "show"); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("group show before allow = %v, want a not-approved refusal", err)
	}
	if _, err := runCmd(t, "", "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}

	out, err := runCmd(t, "", "group", "show")
	if err != nil {
		t.Fatalf("group show without -g: %v", err)
	}
	if !strings.Contains(out, "API_KEY") {
		t.Fatalf("group show without -g = %q, want the project's warden group", out)
	}
}

// Outside a project and without -g there is no group to edit, so the command
// says what to pass rather than guessing one.
func TestGroupCommandsNeedAGroupOutsideAProject(t *testing.T) {
	testEnv(t)
	_, err := runCmd(t, "", "group", "show")
	if err == nil || !strings.Contains(err.Error(), "-g GROUP") {
		t.Fatalf("group show with no group = %v, want a pointer to -g", err)
	}
}

// Only add-group creates a group, so a mistyped -g cannot start one.
func TestGroupSetRefusesAMissingGroup(t *testing.T) {
	testEnv(t)
	_, err := runCmd(t, "sk-test-value", "group", "set", "-g", "nope", "API_KEY")
	if err == nil || !strings.Contains(err.Error(), "add-group") {
		t.Fatalf("group set into a missing group = %v, want an add-group refusal", err)
	}
}

// add never overwrites a flat secret silently: rotate changes a value, and
// --replace is the explicit overwrite.
func TestAddRefusesAnExistingSecret(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-one", "add", "openai"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "sk-test-two", "add", "openai")
	if err == nil || !strings.Contains(err.Error(), "rotate") {
		t.Fatalf("add over an existing secret = %v, want a rotate pointer", err)
	}
	if _, err := runCmd(t, "sk-test-two", "add", "openai", "--replace"); err != nil {
		t.Fatalf("add --replace: %v", err)
	}
}

// add-group with no fields makes an empty group; exec and export then say so
// rather than succeeding silently with nothing injected.
func TestEmptyGroupCanBeCreatedAndSaysItInjectsNothing(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "", "add-group", "bare", "--global"); err != nil {
		t.Fatalf("add-group with no fields: %v", err)
	}

	// runCmd discards stderr, so drive the root command directly to read the
	// warning that export writes there.
	var stdout, stderr bytes.Buffer
	root := newRootCmd(newApp())
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"export", "bare"})
	if err := root.Execute(); err != nil {
		t.Fatalf("export of an empty group: %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("export of an empty group printed %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no variables yet") {
		t.Fatalf("no warning for an empty group: %q", stderr.String())
	}
}
