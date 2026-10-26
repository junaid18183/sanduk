package vault

// Entry scopes. A scope is a view concept: it decides where an entry appears in
// a listing and nothing else. No scope ever changes which entry a command reads
// or which variables it injects — exec and export resolve exactly what they
// resolved before.
const (
	// ScopeProject is an entry named by an approved project file.
	ScopeProject = "project"
	// ScopeMember is a flat secret some group links to. It is a building
	// block of that group rather than something to reach for directly.
	ScopeMember = "member"
	// ScopeGlobal is everything else: an entry that stands on its own.
	ScopeGlobal = "global"
)

// Scoped is one classified entry.
type Scoped struct {
	Name  string
	Scope string
	// Owners is what claimed the entry: the directory it belongs to for
	// ScopeProject, the groups linking to it for ScopeMember, nil otherwise.
	Owners []string
	// Current marks an entry the working directory itself resolves to — any
	// layer of its project chain — as opposed to one belonging to a project
	// elsewhere.
	Current bool
}

// ScopeEntries classifies names for display.
//
// projects maps an entry name to the directory it belongs to, as recorded by
// GroupLocations; current lists the entries the working directory resolves
// to — every layer of its project chain, not just the nearest, since all of
// them inject here. Membership comes from the config's linked_by hints, so
// classification reads no values and opens no backend entry: a listing stays
// as cheap as it is today.
//
// Names are returned in the order given.
func ScopeEntries(names []string, c *Config, projects map[string]string, current []string) []Scoped {
	here := make(map[string]bool, len(current))
	for _, n := range current {
		if n != "" {
			here[n] = true
		}
	}

	out := make([]Scoped, 0, len(names))
	for _, name := range names {
		s := Scoped{Name: name, Scope: ScopeGlobal}
		switch {
		case here[name]:
			s.Scope, s.Current = ScopeProject, true
			if p, ok := projects[name]; ok {
				s.Owners = []string{p}
			}
		case projects[name] != "":
			s.Scope = ScopeProject
			s.Owners = []string{projects[name]}
		default:
			if c != nil {
				if m, ok := c.Secrets[name]; ok && len(m.LinkedBy) > 0 {
					s.Scope = ScopeMember
					s.Owners = append([]string(nil), m.LinkedBy...)
				}
			}
		}
		out = append(out, s)
	}
	return out
}

// ValidScope reports whether s names a scope, for flag validation.
func ValidScope(s string) bool {
	switch s {
	case ScopeProject, ScopeMember, ScopeGlobal:
		return true
	}
	return false
}
