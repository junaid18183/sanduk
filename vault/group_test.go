package vault

import (
	"errors"
	"testing"

	"github.com/99designs/keyring"
)

func TestMergeGroupCreatesWhenAbsent(t *testing.T) {
	v := newFileVault(t)

	if err := v.MergeGroup("warden", map[string]string{"DB_URL": "postgres://x"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	e, err := v.Get("warden")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if e.Type != TypeGroup || e.Fields["DB_URL"] != "postgres://x" {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestMergeGroupLeavesOtherFieldsAlone(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"DB_URL": "one", "API_KEY": "two"}); err != nil {
		t.Fatal(err)
	}

	if err := v.MergeGroup("warden", map[string]string{"DB_URL": "rotated"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	e, err := v.Get("warden")
	if err != nil {
		t.Fatal(err)
	}
	if e.Fields["DB_URL"] != "rotated" {
		t.Fatalf("DB_URL = %q, want rotated", e.Fields["DB_URL"])
	}
	if e.Fields["API_KEY"] != "two" {
		t.Fatalf("API_KEY = %q, want it untouched", e.Fields["API_KEY"])
	}
}

func TestMergeGroupRejectsFlatSecret(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("openai", "sk-test-value"); err != nil {
		t.Fatal(err)
	}

	if err := v.MergeGroup("openai", map[string]string{"A": "1"}); err == nil {
		t.Fatal("expected merging into a flat secret to fail")
	}
}

func TestMergeGroupRejectsNoFields(t *testing.T) {
	v := newFileVault(t)
	if err := v.MergeGroup("warden", nil); err == nil {
		t.Fatal("expected an error for an empty merge")
	}
}

func TestUnsetGroupFields(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}

	if err := v.UnsetGroupFields("warden", []string{"A"}); err != nil {
		t.Fatalf("unset: %v", err)
	}

	names, err := v.GroupFieldNames("warden")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "B" {
		t.Fatalf("fields = %v, want [B]", names)
	}
}

// A group may be emptied: it stays a valid entry, and fields can be added back.
func TestUnsetGroupFieldsCanEmptyAGroup(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := v.UnsetGroupFields("warden", []string{"A"}); err != nil {
		t.Fatalf("unset of the last field: %v", err)
	}
	e, err := v.Get("warden")
	if err != nil {
		t.Fatalf("entry should still exist: %v", err)
	}
	if e.Type != TypeGroup || e.FieldCount() != 0 {
		t.Fatalf("entry = %+v, want an empty group", e)
	}
	if err := v.MergeGroup("warden", map[string]string{"B": "2"}); err != nil {
		t.Fatalf("refill: %v", err)
	}
}

// add-group with no fields creates an empty group.
func TestAddEmptyGroup(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("bare", nil); err != nil {
		t.Fatalf("AddGroup with no fields: %v", err)
	}
	e, err := v.Get("bare")
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != TypeGroup || e.FieldCount() != 0 {
		t.Fatalf("entry = %+v, want an empty group", e)
	}
}

func TestUnsetGroupFieldsUnknownKey(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}

	if err := v.UnsetGroupFields("warden", []string{"NOPE"}); err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

func TestGroupFieldNamesSorted(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"C": "3", "A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}

	names, err := v.GroupFieldNames("warden")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A", "B", "C"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}

func TestGroupFieldNamesRejectsFlatSecret(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("openai", "sk-test-value"); err != nil {
		t.Fatal(err)
	}

	if _, err := v.GroupFieldNames("openai"); err == nil {
		t.Fatal("expected an error for a flat secret")
	}
}

func TestGroupFieldNamesMissing(t *testing.T) {
	v := newFileVault(t)
	if _, err := v.GroupFieldNames("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// countingKeyring wraps a keyring and counts reads, so a command's prompt
// count on a backend that asks per read can be asserted.
type countingKeyring struct {
	keyring.Keyring
	gets map[string]int
}

func (c *countingKeyring) Get(key string) (keyring.Item, error) {
	c.gets[key]++
	return c.Keyring.Get(key)
}

// A dedicated keychain and keychain-biometric both prompt per read, so
// reading one entry twice to learn two things about it costs the user two
// prompts. Variables answers both from a single read.
func TestVariablesReadsTheEntryOnce(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("openai", "sk-test-value"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddGroup("warden", map[string]string{"DB_URL": "postgres://test"}); err != nil {
		t.Fatal(err)
	}
	if err := v.LinkGroupField("warden", "OPENAI_API_KEY", "openai"); err != nil {
		t.Fatal(err)
	}

	counter := &countingKeyring{Keyring: v.kr, gets: map[string]int{}}
	v.kr = counter

	vars, err := v.Variables("warden", &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if n := counter.gets["warden"]; n != 1 {
		t.Fatalf("Variables read the entry %d times, want 1", n)
	}
	if len(vars.Names) != 2 || vars.Links["OPENAI_API_KEY"] != "openai" {
		t.Fatalf("Variables = %+v, want both names and the link from one read", vars)
	}
	// A link target is not read: only names are wanted, and reading it would
	// be another prompt for a value nobody asked for.
	if n := counter.gets["openai"]; n != 0 {
		t.Fatalf("Variables read the link target %d times, want 0", n)
	}
}
