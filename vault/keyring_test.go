package vault

import (
	"testing"

	"github.com/99designs/keyring"
)

// Regression guard for the bug where the library's file backend was allowed to
// shadow a missing native keystore. It never fails to "open" and then errors on
// the first read with "No directory provided for file keyring".
func TestNativeBackendsExcludesFileBackend(t *testing.T) {
	backends := nativeBackends()
	if backends == nil {
		t.Fatal("nativeBackends must never be nil, or keyring.Open falls back to its own defaults")
	}
	for _, b := range backends {
		if b == keyring.FileBackend {
			t.Fatalf("nativeBackends returned the library file backend: %v", backends)
		}
	}
}

func TestOpenKeyringFileBackendIsAge(t *testing.T) {
	t.Setenv("SANDUK_BACKEND", "file")
	t.Setenv("SANDUK_FILE_DIR", t.TempDir())
	t.Setenv("SANDUK_FILE_PASSPHRASE", "test-passphrase")

	kr, err := OpenKeyring()
	if err != nil {
		t.Fatalf("OpenKeyring: %v", err)
	}
	if _, ok := kr.(*ageKeyring); !ok {
		t.Fatalf("file backend is %T, want *ageKeyring", kr)
	}
}

func TestAgePathLocation(t *testing.T) {
	t.Setenv("SANDUK_FILE_DIR", "/tmp/sanduk-path-test")
	if got, want := agePath("sanduk"), "/tmp/sanduk-path-test/sanduk.age"; got != want {
		t.Fatalf("agePath = %q, want %q", got, want)
	}
}

func TestUnknownBackendIsRejected(t *testing.T) {
	t.Setenv("SANDUK_BACKEND", "definitely-not-a-backend")
	if _, err := OpenKeyring(); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestKeychainTarget(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", defaultKeychainName, false},
		{"  ", defaultKeychainName, false},
		{"sanduk", "sanduk", false},
		{"work-sanduk", "work-sanduk", false},
		// keyring appends ".keychain" and hands the result to the Security
		// framework, so a path-shaped value must never be accepted.
		{"../evil", "", true},
		{"a/b", "", true},
		{`a\b`, "", true},
		{".", "", true},
		{"..", "", true},
	}

	for _, c := range cases {
		t.Setenv("SANDUK_KEYCHAIN", c.in)
		got, err := keychainTarget()
		if c.wantErr {
			if err == nil {
				t.Errorf("keychainTarget(%q) = %q, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("keychainTarget(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("keychainTarget(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
