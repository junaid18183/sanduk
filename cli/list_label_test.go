package cli

import (
	"path/filepath"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// A project group owned by another directory names its owner, so a listing
// from elsewhere does not read as if it belonged to the folder you are in.
func TestScopeLabelNamesTheOwner(t *testing.T) {
	// os.UserHomeDir reads $HOME on Unix and %USERPROFILE% on Windows.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	owner := filepath.Join(home, "work", "bidops")

	cases := []struct {
		name string
		e    vault.Scoped
		want string
	}{
		{"owned elsewhere", vault.Scoped{Scope: vault.ScopeProject, Owners: []string{owner}}, "project (" + filepath.Join("~", "work", "bidops") + ")"},
		{"owned here", vault.Scoped{Scope: vault.ScopeProject, Current: true, Owners: []string{owner}}, "project (here)"},
		{"global", vault.Scoped{Scope: vault.ScopeGlobal}, "global"},
		{"member", vault.Scoped{Scope: vault.ScopeMember, Owners: []string{"contexthub"}}, "member of contexthub"},
	}
	for _, c := range cases {
		if got := scopeLabel(c.e); got != c.want {
			t.Errorf("%s: scopeLabel = %q, want %q", c.name, got, c.want)
		}
	}
}
