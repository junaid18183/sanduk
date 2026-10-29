package vault

import (
	"errors"
	"strings"
	"testing"
)

// fieldLinkedVault has an ElevenLabs group holding two keys, and a maktub group
// that does not yet link to either.
func fieldLinkedVault(t *testing.T) *Vault {
	t.Helper()
	v := newFileVault(t)
	if err := v.AddGroup("ElevenLabs", map[string]string{
		"elevenlabs_siddik": "sk-test-siddik",
		"elevenlabs_juned":  "sk-test-juned",
	}); err != nil {
		t.Fatal(err)
	}
	if err := v.AddGroup("maktub", map[string]string{"MAKTUB_DB_URL": "postgres://x"}); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFieldLinkResolvesToTheGroupsStoredValue(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatalf("link: %v", err)
	}

	env, err := v.ResolveEnv("maktub", &Config{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if env["ELEVENLABS_API_KEY"] != "sk-test-siddik" {
		t.Fatalf("field link value = %q", env["ELEVENLABS_API_KEY"])
	}
	if env["MAKTUB_DB_URL"] != "postgres://x" {
		t.Fatalf("stored field was disturbed: %q", env["MAKTUB_DB_URL"])
	}
}

func TestFieldLinkFollowsARotation(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatal(err)
	}
	if err := v.MergeGroup("ElevenLabs", map[string]string{"elevenlabs_siddik": "sk-test-rotated"}); err != nil {
		t.Fatal(err)
	}

	env, err := v.ResolveEnv("maktub", &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if env["ELEVENLABS_API_KEY"] != "sk-test-rotated" {
		t.Fatalf("group did not follow the rotation: %q", env["ELEVENLABS_API_KEY"])
	}
}

// A field link must not chain: the source has to be a value the other group
// stores itself. Otherwise a link could reach a value only through another link.
func TestFieldLinkRefusesALinkedSource(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.AddSecret("elevenlabs", "sk-test-flat"); err != nil {
		t.Fatal(err)
	}
	if err := v.LinkGroupField("ElevenLabs", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "ElevenLabs/ELEVENLABS_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "is a link, not a stored value") {
		t.Fatalf("linking to a link: err = %v", err)
	}
}

func TestFieldLinkRefusesMissingFieldAndNonGroup(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.AddSecret("elevenlabs", "sk-test-flat"); err != nil {
		t.Fatal(err)
	}

	if err := v.LinkGroupField("maktub", "A", "ElevenLabs/nope"); err == nil {
		t.Fatal("linked to a field the group does not store")
	}
	if err := v.LinkGroupField("maktub", "A", "nosuchgroup/x"); err == nil ||
		!errors.Is(err, ErrNotFound) {
		t.Fatalf("linked to a missing group: err = %v", err)
	}
	if err := v.LinkGroupField("maktub", "A", "elevenlabs/x"); err == nil ||
		!strings.Contains(err.Error(), "must point at a group") {
		t.Fatalf("linked to a flat secret as a group: err = %v", err)
	}
	if err := v.LinkGroupField("maktub", "A", "maktub/MAKTUB_DB_URL"); err == nil ||
		!strings.Contains(err.Error(), "cannot link to its own field") {
		t.Fatalf("self-link: err = %v", err)
	}
}

// A group cannot link a variable it already stores as a value.
func TestFieldLinkRefusesAStoredVariable(t *testing.T) {
	v := fieldLinkedVault(t)
	err := v.LinkGroupField("maktub", "MAKTUB_DB_URL", "ElevenLabs/elevenlabs_siddik")
	if err == nil || !strings.Contains(err.Error(), "already stores") {
		t.Fatalf("err = %v", err)
	}
}

func TestFieldLinkUnlinkLeavesTheSourceAlone(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatal(err)
	}
	if err := v.UnlinkGroupField("maktub", []string{"ELEVENLABS_API_KEY"}); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	env, err := v.ResolveEnv("ElevenLabs", &Config{})
	if err != nil {
		t.Fatalf("source group: %v", err)
	}
	if env["elevenlabs_siddik"] != "sk-test-siddik" {
		t.Fatal("unlink changed the source field")
	}
	if _, err := v.ResolveEnv("maktub", &Config{}); err != nil {
		t.Fatalf("maktub after unlink: %v", err)
	}
}

// Removing the source field leaves the link dangling, which must fail closed
// rather than inject nothing.
func TestFieldLinkDanglingFailsClosed(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "ElevenLabs/elevenlabs_siddik"); err != nil {
		t.Fatal(err)
	}
	if err := v.UnsetGroupFields("ElevenLabs", []string{"elevenlabs_siddik"}); err != nil {
		t.Fatal(err)
	}

	_, err := v.ResolveEnv("maktub", &Config{})
	if err == nil || !strings.Contains(err.Error(), "not a stored field") {
		t.Fatalf("dangling field link: err = %v", err)
	}
}

// A flat secret whose name contains "/" stays linkable only through the
// unchanged flat path, and a field link is never read as one.
func TestFlatTargetWithSlashIsNotAFieldLink(t *testing.T) {
	v := fieldLinkedVault(t)
	if err := v.AddSecret("team/key", "sk-test-slash"); err != nil {
		t.Fatal(err)
	}
	if err := v.LinkGroupField("maktub", "SLASHED", "team/key"); err == nil {
		t.Fatal("a target with a slash was read as a flat secret; it must be read as Group/FIELD")
	}
}
