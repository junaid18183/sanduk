package vault

import (
	"errors"
	"strings"
	"testing"
)

func linkedVault(t *testing.T) *Vault {
	t.Helper()
	v := newFileVault(t)
	if err := v.AddSecret("elevenlabs", "sk-test-one"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddGroup("maktub", map[string]string{"MAKTUB_DB_URL": "postgres://x"}); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLinkResolvesThroughTheTarget(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatalf("link: %v", err)
	}

	env, err := v.ResolveEnv("maktub", &Config{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if env["ELEVENLABS_API_KEY"] != "sk-test-one" {
		t.Fatalf("linked value = %q", env["ELEVENLABS_API_KEY"])
	}
	if env["MAKTUB_DB_URL"] != "postgres://x" {
		t.Fatalf("stored field was disturbed: %q", env["MAKTUB_DB_URL"])
	}
}

// The whole point of a link: the group follows the secret with no second edit.
func TestLinkFollowsARotation(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddSecret("elevenlabs", "sk-test-rotated"); err != nil {
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

func TestLinkNeverCopiesTheValue(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	e, err := v.Get("maktub")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Fields["ELEVENLABS_API_KEY"]; ok {
		t.Fatal("link stored a copy of the value in the group")
	}
	if e.Links["ELEVENLABS_API_KEY"] != "elevenlabs" {
		t.Fatalf("links = %v", e.Links)
	}
}

func TestLinkRefusesAGroupTarget(t *testing.T) {
	v := linkedVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "b"}); err != nil {
		t.Fatal(err)
	}
	err := v.LinkGroupField("maktub", "SOME_KEY", "warden")
	if err == nil || !strings.Contains(err.Error(), "flat secret") {
		t.Fatalf("err = %v, want a flat-secret refusal", err)
	}
}

func TestLinkRefusesSelfReference(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "SOME_KEY", "maktub"); err == nil {
		t.Fatal("expected a self-link refusal")
	}
}

func TestLinkRefusesAMissingTarget(t *testing.T) {
	v := linkedVault(t)
	err := v.LinkGroupField("maktub", "SOME_KEY", "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLinkRefusesAStoredField(t *testing.T) {
	v := linkedVault(t)
	err := v.LinkGroupField("maktub", "MAKTUB_DB_URL", "elevenlabs")
	if err == nil || !strings.Contains(err.Error(), "already stores") {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestLinkRejectsAnInvalidEnvName(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "not-a-name", "elevenlabs"); err == nil {
		t.Fatal("expected an env-name rejection")
	}
}

// Only add-group creates a group, so a link into a missing one is refused and
// nothing is written.
func TestLinkRefusesAMissingGroup(t *testing.T) {
	v := linkedVault(t)
	err := v.LinkGroupField("fresh", "ELEVENLABS_API_KEY", "elevenlabs")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("link into a missing group: err = %v", err)
	}
	if ok, _ := v.Exists("fresh"); ok {
		t.Fatal("a refused link still created the group")
	}
}

// A removed target must fail loudly rather than leave the variable absent.
func TestDanglingLinkFailsClosed(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove("elevenlabs"); err != nil {
		t.Fatal(err)
	}

	_, err := v.ResolveEnv("maktub", &Config{})
	if err == nil {
		t.Fatal("expected a dangling link to fail")
	}
	for _, want := range []string{"maktub", "ELEVENLABS_API_KEY", "elevenlabs"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
}

func TestUnlinkLeavesTheSecretAlone(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	if err := v.UnlinkGroupField("maktub", []string{"ELEVENLABS_API_KEY"}); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if _, err := v.Get("elevenlabs"); err != nil {
		t.Fatalf("unlink disturbed the secret: %v", err)
	}
	e, err := v.Get("maktub")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Links) != 0 {
		t.Fatalf("links = %v", e.Links)
	}
}

func TestUnlinkCanEmptyAGroup(t *testing.T) {
	v := linkedVault(t)
	if err := v.AddGroup("solo", map[string]string{"SOLO_VAR": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := v.LinkGroupField("solo", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	// Leave the link as the group's only variable, so removing it would empty
	// the entry.
	if err := v.UnsetGroupFields("solo", []string{"SOLO_VAR"}); err != nil {
		t.Fatal(err)
	}
	if err := v.UnlinkGroupField("solo", []string{"ELEVENLABS_API_KEY"}); err != nil {
		t.Fatalf("unlink of the last link: %v", err)
	}
	if ok, _ := v.Exists("solo"); !ok {
		t.Fatal("unlinking the last link removed the group")
	}
}

func TestUnsetAndUnlinkPointAtEachOther(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	err := v.UnsetGroupFields("maktub", []string{"ELEVENLABS_API_KEY"})
	if err == nil || !strings.Contains(err.Error(), "group unlink") {
		t.Fatalf("unset of a link: %v, want a pointer to unlink", err)
	}
	err = v.UnlinkGroupField("maktub", []string{"MAKTUB_DB_URL"})
	if err == nil || !strings.Contains(err.Error(), "group unset") {
		t.Fatalf("unlink of a stored field: %v, want a pointer to unset", err)
	}
}

func TestGroupFieldNamesIncludesLinks(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}
	names, err := v.GroupFieldNames("maktub")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "ELEVENLABS_API_KEY" || names[1] != "MAKTUB_DB_URL" {
		t.Fatalf("names = %v", names)
	}
}

func TestGroupFieldValueReportsTheSource(t *testing.T) {
	v := linkedVault(t)
	if err := v.LinkGroupField("maktub", "ELEVENLABS_API_KEY", "elevenlabs"); err != nil {
		t.Fatal(err)
	}

	val, linked, err := v.GroupFieldValue("maktub", "ELEVENLABS_API_KEY")
	if err != nil || val != "sk-test-one" || linked != "elevenlabs" {
		t.Fatalf("val=%q linked=%q err=%v", val, linked, err)
	}
	val, linked, err = v.GroupFieldValue("maktub", "MAKTUB_DB_URL")
	if err != nil || val != "postgres://x" || linked != "" {
		t.Fatalf("val=%q linked=%q err=%v", val, linked, err)
	}
}

// A hand-edited backend must not be able to produce an entry that injects an
// invalid variable name or a secret carrying links.
func TestStoredLinksAreValidatedOnRead(t *testing.T) {
	for name, e := range map[string]*Entry{
		"bad env name":   {Version: 1, Type: TypeGroup, Links: map[string]string{"not-a-name": "elevenlabs"}},
		"empty target":   {Version: 1, Type: TypeGroup, Links: map[string]string{"OK_NAME": ""}},
		"field and link": {Version: 1, Type: TypeGroup, Fields: map[string]string{"K": "v"}, Links: map[string]string{"K": "elevenlabs"}},
		"secret w/ link": {Version: 1, Type: TypeSecret, Value: "v", Links: map[string]string{"K": "elevenlabs"}},
	} {
		if _, err := e.Marshal(); err == nil {
			t.Fatalf("%s: expected a rejection", name)
		}
	}
}
