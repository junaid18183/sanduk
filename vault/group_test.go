package vault

import (
	"errors"
	"testing"
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

func TestUnsetGroupFieldsRefusesLastField(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("warden", map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}

	if err := v.UnsetGroupFields("warden", []string{"A"}); err == nil {
		t.Fatal("expected removing the last field to be refused")
	}

	// The entry must be untouched after the refusal.
	e, err := v.Get("warden")
	if err != nil {
		t.Fatalf("entry should still exist: %v", err)
	}
	if e.Fields["A"] != "1" {
		t.Fatalf("fields = %v, want them unchanged", e.Fields)
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
