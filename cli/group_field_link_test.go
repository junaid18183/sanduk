package cli

import (
	"strings"
	"testing"
)

// seedFieldLinkable has an ElevenLabs group holding a key, and a maktub group
// that wants it under a single variable name.
func seedFieldLinkable(t *testing.T) {
	t.Helper()
	testEnv(t)
	if _, err := runCmd(t, `{"elevenlabs_siddik":"sk-test-siddik"}`, "add-group", "ElevenLabs", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, `{"MAKTUB_DB_URL":"postgres://x"}`, "add-group", "maktub", "--from-json", "-"); err != nil {
		t.Fatal(err)
	}
}

func TestGroupLinkToAGroupField(t *testing.T) {
	seedFieldLinkable(t)

	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "ELEVENLABS_API_KEY=ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatalf("link: %v", err)
	}

	out, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(out, "ELEVENLABS_API_KEY=sk-test-siddik") {
		t.Fatalf("export = %q", out)
	}
}

// A field target has no secret name to take the variable from, so a bare
// target is refused rather than guessed at.
func TestGroupLinkFieldNeedsAVariableName(t *testing.T) {
	seedFieldLinkable(t)

	_, err := runCmd(t, "", "group", "link", "-g", "maktub", "ElevenLabs/elevenlabs_siddik")
	if err == nil || !strings.Contains(err.Error(), "give the variable too") {
		t.Fatalf("bare field target: err = %v", err)
	}
}

// A field link is a group, not a flat secret, so it must not leave a linked_by
// hint behind that would misplace the group in a listing.
func TestGroupLinkToAGroupFieldRecordsNoHint(t *testing.T) {
	seedFieldLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "ELEVENLABS_API_KEY=ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatal(err)
	}

	cfg, err := runCmd(t, "", "config")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if strings.Contains(cfg, "ElevenLabs/elevenlabs_siddik") {
		t.Fatalf("config recorded a hint for a field target: %q", cfg)
	}
}

func TestGroupUnlinkDropsAFieldLink(t *testing.T) {
	seedFieldLinkable(t)
	if _, err := runCmd(t, "", "group", "link", "-g", "maktub", "ELEVENLABS_API_KEY=ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "group", "unlink", "-g", "maktub", "ELEVENLABS_API_KEY"); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	out, err := runCmd(t, "", "export", "maktub", "--format", "dotenv")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if strings.Contains(out, "ELEVENLABS_API_KEY") {
		t.Fatalf("unlinked variable still exported: %q", out)
	}
}
