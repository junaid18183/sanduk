package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
)

// selector names what to resolve into environment variables. It is either a
// bare entry name — resolved through the entry's own mapping, so a group
// contributes one variable per field — or an ENVVAR=entry binding, which
// exposes a single flat secret under the given name.
type selector struct {
	env  string
	name string
}

// parseSelector interprets an exec/export selector. A "=" makes it a binding
// when both sides are non-empty, which is the long-standing behaviour.
func parseSelector(s string) selector {
	if env, name, ok := strings.Cut(s, "="); ok && env != "" && name != "" {
		return selector{env: env, name: name}
	}
	return selector{name: s}
}

// parseSelectors converts raw arguments into selectors.
func parseSelectors(args []string) []selector {
	out := make([]selector, 0, len(args))
	for _, a := range args {
		out = append(out, parseSelector(a))
	}
	return out
}

// resolveSelection is the pipeline exec and export share: open the vault, fall
// back to the approved project file when no selector is given, resolve the
// selectors left to right, then apply --only and --except. The project file's
// `only` applies only when --only was not given.
func resolveSelection(sels, only, except []string) (map[string]string, error) {
	v, err := openVault()
	if err != nil {
		return nil, err
	}
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, err
	}

	if len(sels) == 0 {
		entry, projOnly, err := defaultEntry()
		if err != nil {
			return nil, err
		}
		sels = []string{entry}
		if len(only) == 0 {
			only = projOnly
		}
	}

	env, err := resolveSelectors(v, cfg, parseSelectors(sels))
	if err != nil {
		return nil, err
	}
	env, err = filterOnly(env, only)
	if err != nil {
		return nil, err
	}
	return filterExcept(env, except), nil
}

// resolveSelectors expands selectors left to right and merges the results. A
// later selector wins when two of them produce the same variable, so a base
// entry can be overridden by a narrower one.
func resolveSelectors(v *vault.Vault, cfg *vault.Config, sels []selector) (map[string]string, error) {
	env := make(map[string]string)
	for _, s := range sels {
		part, err := resolveOne(v, cfg, s)
		if err != nil {
			return nil, err
		}
		for k, val := range part {
			env[k] = val
		}
	}
	return env, nil
}

func resolveOne(v *vault.Vault, cfg *vault.Config, s selector) (map[string]string, error) {
	if s.env != "" {
		e, err := v.Get(s.name)
		if err != nil {
			return nil, err
		}
		if e.Type != vault.TypeSecret {
			return nil, fmt.Errorf("%q is a group; ad-hoc binding requires a flat secret", s.name)
		}
		return map[string]string{s.env: e.Value}, nil
	}
	return v.ResolveEnv(s.name, cfg)
}

// filterOnly narrows env to the named keys. An unknown key is an error rather
// than a silent omission: a typo in a least-privilege allowlist should fail
// loudly instead of quietly running a process without the secret it needs.
func filterOnly(env map[string]string, only []string) (map[string]string, error) {
	keys := splitOnly(only)
	if len(keys) == 0 {
		return env, nil
	}

	out := make(map[string]string, len(keys))
	var missing []string
	for _, k := range keys {
		val, ok := env[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		out[k] = val
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no such key in the selection: %s (available: %s)",
			quoteList(missing), quoteList(sortedMapKeys(env)))
	}
	return out, nil
}

// filterExcept removes the named keys from env. Unlike --only, an unknown key is
// not an error: excluding something that is not present is a harmless no-op,
// whereas a typo in an allowlist would silently drop a secret that is needed.
func filterExcept(env map[string]string, except []string) map[string]string {
	keys := splitOnly(except)
	if len(keys) == 0 {
		return env
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if drop[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// splitOnly flattens repeated --only flags, each of which may hold a
// comma-separated list.
func splitOnly(flags []string) []string {
	var out []string
	for _, f := range flags {
		for _, k := range strings.Split(f, ",") {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
	}
	return out
}

// quoteList renders names for an error message, sorted and quoted so an empty
// name or a stray space is visible.
func quoteList(items []string) string {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	quoted := make([]string, len(sorted))
	for i, s := range sorted {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}
