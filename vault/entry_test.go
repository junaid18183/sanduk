package vault

import (
	"errors"
	"testing"
)

func TestEntryMarshalSecret(t *testing.T) {
	b, err := (&Entry{Type: TypeSecret, Value: "sk-test"}).Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	e, err := UnmarshalEntry(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Type != TypeSecret || e.Value != "sk-test" {
		t.Fatalf("roundtrip mismatch: %+v", e)
	}
	if e.Version != entryVersion {
		t.Fatalf("version = %d, want %d", e.Version, entryVersion)
	}
}

func TestEntryMarshalGroup(t *testing.T) {
	in := &Entry{Type: TypeGroup, Fields: map[string]string{"A": "1", "B": "2"}}
	b, err := in.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	e, err := UnmarshalEntry(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Fields) != 2 || e.Fields["A"] != "1" {
		t.Fatalf("roundtrip mismatch: %+v", e)
	}
}

func TestEntryValidation(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
	}{
		{"unknown type", Entry{Type: "nope"}},
		{"empty secret", Entry{Type: TypeSecret}},
		{"empty group", Entry{Type: TypeGroup, Fields: map[string]string{}}},
	}
	for _, tc := range cases {
		if _, err := tc.e.Marshal(); err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestUnmarshalEntryCorrupt(t *testing.T) {
	if _, err := UnmarshalEntry([]byte("not json")); err == nil {
		t.Fatal("expected error for corrupt entry")
	}
}

func TestRedactNeverRevealsSecret(t *testing.T) {
	got := Redact("sk-abcdefgh1234")
	if got == "sk-abcdefgh1234" {
		t.Fatal("redact returned the raw secret")
	}
	if want := "****1234"; got != want {
		t.Fatalf("Redact = %q, want %q", got, want)
	}
	if short := Redact("ab"); short != "**" {
		t.Fatalf("Redact short = %q, want **", short)
	}
}

func TestErrNotFoundIs(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("sentinel should match itself")
	}
}
