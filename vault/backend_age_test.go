package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/keyring"
)

func newTestAgeKeyring(t *testing.T) (*ageKeyring, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.age")
	k := newAgeKeyring(path, func(string) (string, error) { return "[REDACTED]", nil })
	return k, path
}

func TestAgeKeyringLifecycle(t *testing.T) {
	k, _ := newTestAgeKeyring(t)

	if _, err := k.Get("missing"); err != keyring.ErrKeyNotFound {
		t.Fatalf("Get(missing) = %v, want ErrKeyNotFound", err)
	}

	if err := k.Set(keyring.Item{Key: "openai", Data: []byte("sk-secret-value")}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := k.Get("openai")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Data) != "sk-secret-value" {
		t.Fatalf("Data = %q", got.Data)
	}

	keys, err := k.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "openai" {
		t.Fatalf("Keys = %v", keys)
	}

	if err := k.Remove("openai"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := k.Remove("openai"); err != keyring.ErrKeyNotFound {
		t.Fatalf("second Remove = %v, want ErrKeyNotFound", err)
	}
}

func TestAgeKeyringFileIsEncrypted(t *testing.T) {
	k, path := newTestAgeKeyring(t)

	const secret = "sk-plaintext-must-not-appear"
	if err := k.Set(keyring.Item{Key: "openai", Data: []byte(secret)}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vault file: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("plaintext secret found in the vault file")
	}
	if !strings.Contains(string(raw), "age-encryption.org") {
		t.Fatal("file does not look like an age-encrypted document")
	}
}

func TestAgeKeyringPersistenceAcrossInstances(t *testing.T) {
	k, path := newTestAgeKeyring(t)
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err != nil {
		t.Fatal(err)
	}

	reopened := newAgeKeyring(path, func(string) (string, error) { return "[REDACTED]", nil })
	got, err := reopened.Get("a")
	if err != nil {
		t.Fatalf("reopen Get: %v", err)
	}
	if string(got.Data) != "1" {
		t.Fatalf("Data = %q", got.Data)
	}
}

func TestAgeKeyringWrongPassphrase(t *testing.T) {
	k, path := newTestAgeKeyring(t)
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err != nil {
		t.Fatal(err)
	}

	wrong := newAgeKeyring(path, func(string) (string, error) { return "not-the-passphrase", nil })
	if _, err := wrong.Get("a"); err == nil {
		t.Fatal("expected an error with the wrong passphrase")
	}
}

func TestAgeKeyringMetadata(t *testing.T) {
	k, _ := newTestAgeKeyring(t)
	if err := k.Set(keyring.Item{Key: "openai", Label: "sanduk: openai", Data: []byte("sk-x")}); err != nil {
		t.Fatal(err)
	}
	meta, err := k.GetMetadata("openai")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if meta.Item == nil || meta.Item.Label != "sanduk: openai" {
		t.Fatalf("metadata = %+v", meta)
	}
	if len(meta.Item.Data) != 0 {
		t.Fatal("metadata must not include the secret data")
	}
	if _, err := k.GetMetadata("missing"); err != keyring.ErrKeyNotFound {
		t.Fatalf("GetMetadata(missing) = %v", err)
	}
}

func TestAgeKeyringChmodsFile(t *testing.T) {
	k, path := newTestAgeKeyring(t)
	if err := k.Set(keyring.Item{Key: "a", Data: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("vault file perm = %o, want 600", perm)
	}
}
