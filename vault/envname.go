package vault

import (
	"fmt"
	"sort"
	"strings"
)

// validEnvName reports whether name is a portable environment variable name: a
// letter or underscore followed by letters, digits or underscores.
func validEnvName(name string) bool {
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

// sensitiveEnvNames change how the dynamic loader, a shell, an interpreter or
// git behaves when present in a child environment. Injecting one from a vault is
// almost never intended and is a classic way to run code inside the child, so
// any use is refused rather than silently allowed. PATH is deliberately not
// listed: groups legitimately extend it.
var sensitiveEnvNames = map[string]bool{
	// shells
	"BASH_ENV":       true,
	"ENV":            true,
	"PROMPT_COMMAND": true,
	"PS4":            true,
	"SHELLOPTS":      true,
	"BASHOPTS":       true,
	"IFS":            true,
	"ZDOTDIR":        true,
	// glibc
	"GCONV_PATH": true,
	// interpreters and runtimes
	"NODE_OPTIONS":      true,
	"NODE_PATH":         true,
	"PYTHONSTARTUP":     true,
	"PYTHONPATH":        true,
	"PYTHONHOME":        true,
	"PERL5OPT":          true,
	"PERL5LIB":          true,
	"PERLLIB":           true,
	"RUBYOPT":           true,
	"RUBYLIB":           true,
	"JAVA_TOOL_OPTIONS": true,
	"_JAVA_OPTIONS":     true,
	"JDK_JAVA_OPTIONS":  true,
	// git
	"GIT_SSH_COMMAND": true,
	"GIT_SSH":         true,
	"GIT_EXEC_PATH":   true,
	"GIT_ASKPASS":     true,
}

// sensitiveEnvPrefixes cover whole families of variables. LD_ and DYLD_ are the
// dynamic loaders on Linux and macOS. GIT_CONFIG_ (GIT_CONFIG_COUNT,
// GIT_CONFIG_KEY_n, ...) injects arbitrary git configuration, including
// core.sshCommand. SANDUK_ is sanduk's own configuration: a stored SANDUK_TRUST
// or SANDUK_BACKEND would reconfigure a child that is itself sanduk.
var sensitiveEnvPrefixes = []string{"LD_", "DYLD_", "GIT_CONFIG_", sandukEnvPrefix}

// ValidateEnvName checks that a name is safe to inject as an environment
// variable. It is enforced when a name is stored and again whenever one is about
// to reach a child process or be printed by export, so neither a hand-edited
// config nor an entry written by an older sanduk can smuggle a loader-affecting
// or shell-breaking name through.
func ValidateEnvName(name string) error {
	if !validEnvName(name) {
		return fmt.Errorf("invalid environment variable name %q (want [A-Za-z_][A-Za-z0-9_]*)", name)
	}
	u := strings.ToUpper(name)
	if sensitiveEnvNames[u] {
		return fmt.Errorf("refusing %q as an environment variable name: it changes how a child process runs", name)
	}
	for _, p := range sensitiveEnvPrefixes {
		if strings.HasPrefix(u, p) {
			return fmt.Errorf("refusing %q as an environment variable name: %s* variables change how a child process runs", name, p)
		}
	}
	return nil
}

// validateEnvNames applies ValidateEnvName to every key of env, in sorted order
// so the error is deterministic.
func validateEnvNames(env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := ValidateEnvName(k); err != nil {
			return err
		}
	}
	return nil
}
