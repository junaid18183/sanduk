package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"
)

func TestAgeKeyringConfirmsPassphraseOnCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.age")
	calls := 0
	k := newAgeKeyring(path, func(string) (string, error) {
		calls++
		if calls == 1 {
			return "one", nil
		}
		return "two", nil
	})
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err == nil {
		t.Fatal("expected a mismatched confirm on creation to fail")
	}
}

func TestAgeKeyringConfirmMatchesOnCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.age")
	calls := 0
	k := newAgeKeyring(path, func(string) (string, error) { calls++; return "same", nil })
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two prompts while creating the vault, got %d", calls)
	}

	// Reopening an existing vault prompts once, not twice.
	again := 0
	r := newAgeKeyring(path, func(string) (string, error) { again++; return "same", nil })
	if _, err := r.Get("a"); err != nil {
		t.Fatalf("reopen Get: %v", err)
	}
	if again != 1 {
		t.Fatalf("expected one prompt on an existing vault, got %d", again)
	}
}

func TestAgeKeyringCreatesLockFile(t *testing.T) {
	k, path := newTestAgeKeyring(t)
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf("expected a lock file next to the vault: %v", err)
	}
}
