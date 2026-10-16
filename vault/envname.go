package vault

import (
	"fmt"
	"strings"
)

// ValidEnvName reports whether name is a portable environment variable name: a
// letter or underscore followed by letters, digits or underscores.
func ValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			// always allowed
		case i > 0 && c >= '0' && c <= '9':
			// digits are allowed after the first character
		default:
			return false
		}
	}
	return true
}

// sensitiveEnvNames change how the dynamic loader or an interpreter behaves when
// present in a child environment. Injecting one from a vault is almost never
// intended, so a write that would use one is refused rather than silently
// allowed. PATH is deliberately not listed: groups legitimately extend it.
var sensitiveEnvNames = map[string]bool{
	"LD_PRELOAD":            true,
	"LD_LIBRARY_PATH":       true,
	"DYLD_INSERT_LIBRARIES": true,
	"DYLD_LIBRARY_PATH":     true,
	"BASH_ENV":              true,
	"ENV":                   true,
	"NODE_OPTIONS":          true,
	"PYTHONSTARTUP":         true,
	"PERL5OPT":              true,
	"RUBYOPT":               true,
}

// ValidateEnvName checks that a group field name is safe to inject as an
// environment variable. It is enforced on write, so a hand-typed or imported
// field cannot become a loader-affecting variable in a child process.
func ValidateEnvName(name string) error {
	if !ValidEnvName(name) {
		return fmt.Errorf("invalid environment variable name %q (want [A-Za-z_][A-Za-z0-9_]*)", name)
	}
	if sensitiveEnvNames[strings.ToUpper(name)] {
		return fmt.Errorf("refusing %q as an environment variable name: it changes loader behaviour", name)
	}
	return nil
}
