package vault

import (
	"reflect"
	"testing"
)

func scopeOf(got []Scoped, name string) Scoped {
	for _, s := range got {
		if s.Name == name {
			return s
		}
	}
	return Scoped{}
}

func TestScopeEntries(t *testing.T) {
	cfg := &Config{
		Secrets: map[string]SecretMeta{
			"openai":     {Env: "OPENAI_API_KEY", LinkedBy: []string{"agents", "warden"}},
			"standalone": {Env: "STANDALONE"},
		},
		Groups: map[string]GroupMeta{"warden": {}, "agents": {}},
	}
	projects := map[string]string{"warden": "/repo"}
	names := []string{"agents", "openai", "standalone", "warden"}

	got := ScopeEntries(names, cfg, projects, []string{"warden"})

	if len(got) != len(names) {
		t.Fatalf("got %d rows, want %d", len(got), len(names))
	}
	for i, s := range got {
		if s.Name != names[i] {
			t.Fatalf("row %d = %q, want %q: order must be preserved", i, s.Name, names[i])
		}
	}

	if s := scopeOf(got, "warden"); s.Scope != ScopeProject || !s.Current {
		t.Fatalf("warden = %+v, want the current project entry", s)
	}
	// A secret a group links to is that group's building block, not a
	// top-level entry.
	if s := scopeOf(got, "openai"); s.Scope != ScopeMember ||
		!reflect.DeepEqual(s.Owners, []string{"agents", "warden"}) {
		t.Fatalf("openai = %+v, want member of agents and warden", s)
	}
	if s := scopeOf(got, "agents"); s.Scope != ScopeGlobal {
		t.Fatalf("agents = %+v, want global: no project claims it", s)
	}
	if s := scopeOf(got, "standalone"); s.Scope != ScopeGlobal {
		t.Fatalf("standalone = %+v, want global", s)
	}

	// Outside the repository the same entry is still project-scoped, but not
	// the current one.
	out := ScopeEntries(names, cfg, projects, nil)
	if s := scopeOf(out, "warden"); s.Scope != ScopeProject || s.Current {
		t.Fatalf("warden outside its repo = %+v, want project and not current", s)
	}
}

// Classification must not need the config: a vault with no metadata at all
// still lists, every entry global.
func TestScopeEntriesWithoutConfig(t *testing.T) {
	got := ScopeEntries([]string{"a", "b"}, nil, nil, nil)
	for _, s := range got {
		if s.Scope != ScopeGlobal {
			t.Fatalf("%s = %q, want global", s.Name, s.Scope)
		}
	}
}

// The project file wins over a linked_by hint: an entry a repository names is
// what that repository is about, even if some group also links to it.
func TestScopeProjectBeatsMembership(t *testing.T) {
	cfg := &Config{Secrets: map[string]SecretMeta{"token": {LinkedBy: []string{"warden"}}}}
	got := ScopeEntries([]string{"token"}, cfg, map[string]string{"token": "/repo"}, []string{"token"})
	if s := got[0]; s.Scope != ScopeProject {
		t.Fatalf("token = %+v, want project", s)
	}
}

func TestValidScope(t *testing.T) {
	for _, s := range []string{ScopeGlobal, ScopeProject, ScopeMember} {
		if !ValidScope(s) {
			t.Fatalf("%q should be a scope", s)
		}
	}
	for _, s := range []string{"", "all", "auto", "Global"} {
		if ValidScope(s) {
			t.Fatalf("%q should not be a scope", s)
		}
	}
}

// Every layer of a project chain is current: each one injects here, so none
// of them belongs in the long tail a listing hides.
func TestScopeEntriesChainIsAllCurrent(t *testing.T) {
	cfg := &Config{Groups: map[string]GroupMeta{"code": {}, "acme": {}, "bidops": {}}}
	projects := map[string]string{
		"code": "/code", "acme": "/code/acme", "bidops": "/code/acme/bidops",
	}
	got := ScopeEntries([]string{"code", "acme", "bidops", "other"}, cfg, projects,
		[]string{"code", "acme", "bidops"})

	for _, name := range []string{"code", "acme", "bidops"} {
		if s := scopeOf(got, name); s.Scope != ScopeProject || !s.Current {
			t.Fatalf("%s = %+v, want a current project entry", name, s)
		}
	}
	if s := scopeOf(got, "other"); s.Current {
		t.Fatalf("other = %+v, want not current", s)
	}
}
