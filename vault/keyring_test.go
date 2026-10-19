package vault

import (
	"path/filepath"
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
	o := testOptions(t)
	kr, err := openKeyring(o)
	if err != nil {
		t.Fatalf("openKeyring: %v", err)
	}
	ak, ok := kr.(*ageKeyring)
	if !ok {
		t.Fatalf("file backend is %T, want *ageKeyring", kr)
	}
	if want := filepath.Join(o.FileDir, o.Service+".age"); ak.path != want {
		t.Fatalf("age path = %q, want %q", ak.path, want)
	}
}

func TestAgeBackendNeedsFileDir(t *testing.T) {
	o := testOptions(t)
	o.FileDir = ""
	if _, err := openKeyring(o); err == nil {
		t.Fatal("opened the age backend with no file directory")
	}
}

func TestUnknownBackendIsRejected(t *testing.T) {
	o := testOptions(t)
	o.Backend = "definitely-not-a-backend"
	if _, err := openKeyring(o); err == nil {
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
		got, err := keychainTarget(c.in)
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
