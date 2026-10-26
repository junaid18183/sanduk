package cli

import (
	"fmt"
	"path/filepath"
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

// selection is the result of the pipeline: the variables to inject, plus any
// file secrets the caller must materialize into paths. Both are filtered by
// --only and --except, which name variables either way.
type selection struct {
	env   map[string]string
	files map[string]vault.FilePayload
}

// names returns every variable the selection contributes, sorted.
func (s selection) names() []string {
	all := make([]string, 0, len(s.env)+len(s.files))
	for k := range s.env {
		all = append(all, k)
	}
	for k := range s.files {
		all = append(all, k)
	}
	sort.Strings(all)
	return all
}

// resolveSelection is the pipeline exec and export share: open the vault, fall
// back to the approved project file when no selector is given, resolve the
// selectors left to right, then apply --only and --except. The project file's
// `only` applies only when --only was not given.
func (a *app) resolveSelection(sels, only, except []string) (selection, error) {
	v, err := a.openVault()
	if err != nil {
		return selection{}, err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return selection{}, err
	}

	r := newResolver(v, cfg)
	if len(sels) == 0 {
		layers, err := a.defaultProjects()
		if err != nil {
			return selection{}, err
		}
		return r.resolveProjects(layers, only, except)
	}

	sel, err := r.resolveSelectors(parseSelectors(sels))
	if err != nil {
		return selection{}, err
	}
	sel, err = filterOnly(sel, only)
	if err != nil {
		return selection{}, err
	}
	return filterExcept(sel, except), nil
}

// resolveProjects resolves a project chain: every layer in turn, outermost
// first, each contributing the entries it includes merged underneath its own
// entry. A later layer wins on a variable two of them provide, so the nearest
// directory has the last word — the same left-to-right rule as
// "sanduk exec a b c", applied down the directory tree.
//
// Each layer's `only` narrows that layer's own entry and nothing else. The
// includes are deliberately not narrowed: `only` lists the keys the layer's
// own group provides, and sanduk keeps it in step with that group, so
// narrowing an include with it would drop every variable the include
// contributes the moment the list was synced, silently. Narrowing the layers
// below would be worse still — a repository's `only` would silently delete
// what the directories above it share. --only on the command line is one
// explicit request, so it still narrows the merged result, and it replaces
// every layer's list while it is given.
func (r *resolver) resolveProjects(layers []projectLayer, only, except []string) (selection, error) {
	out := selection{env: map[string]string{}, files: map[string]vault.FilePayload{}}
	for _, l := range layers {
		// Which file asked for the entry, since with a chain it may be a
		// directory the user is not standing in.
		where := filepath.Join(l.Dir, vault.ProjectFileName)
		base, err := r.resolveSelectors(parseSelectors(l.Include))
		if err != nil {
			return selection{}, fmt.Errorf("%s: %w", where, err)
		}
		own, err := r.resolveSelectors(parseSelectors([]string{l.Entry}))
		if err != nil {
			return selection{}, fmt.Errorf("%s: %w", where, err)
		}
		if len(only) == 0 && len(l.Only) > 0 {
			if own, err = filterOnly(own, l.Only); err != nil {
				return selection{}, fmt.Errorf("%s: %w", where, err)
			}
		}
		out = merge(out, merge(base, own))
	}
	if len(only) > 0 {
		var err error
		if out, err = filterOnly(out, only); err != nil {
			return selection{}, err
		}
	}
	return filterExcept(out, except), nil
}

// merge layers one selection over another, last wins, across kinds: a plain
// variable replaces a file secret's path and the reverse.
func merge(into, over selection) selection {
	for k, val := range over.env {
		into.env[k] = val
		delete(into.files, k)
	}
	for k, p := range over.files {
		into.files[k] = p
		delete(into.env, k)
	}
	return into
}

// resolver expands selectors against one vault, remembering what each entry
// produced.
//
// A chain of project files repeats itself: every layer may include the same
// shared entry, and resolving it again would read it again — which on a
// presence-gated backend is a second Touch ID prompt for a value sanduk
// already holds. The cache is read-only to its callers: a cached selection is
// only ever merged *over* another, and merge mutates the selection underneath,
// never the one on top.
type resolver struct {
	v     *vault.Vault
	cfg   *vault.Config
	cache map[selector]selection
}

func newResolver(v *vault.Vault, cfg *vault.Config) *resolver {
	return &resolver{v: v, cfg: cfg, cache: map[selector]selection{}}
}

// resolveSelectors expands selectors left to right and merges the results. A
// later selector wins when two of them produce the same variable, so a base
// entry can be overridden by a narrower one — including across kinds, so a
// plain variable can override a file secret's path and the reverse.
func (r *resolver) resolveSelectors(sels []selector) (selection, error) {
	out := selection{env: map[string]string{}, files: map[string]vault.FilePayload{}}
	for _, s := range sels {
		got, ok := r.cache[s]
		if !ok {
			env, files, err := resolveOne(r.v, r.cfg, s)
			if err != nil {
				return selection{}, err
			}
			got = selection{env: env, files: files}
			r.cache[s] = got
		}
		out = merge(out, got)
	}
	return out, nil
}

func resolveOne(v *vault.Vault, cfg *vault.Config, s selector) (map[string]string, map[string]vault.FilePayload, error) {
	if s.env != "" {
		e, err := v.Get(s.name)
		if err != nil {
			return nil, nil, err
		}
		switch e.Type {
		case vault.TypeSecret:
			return map[string]string{s.env: e.Value}, nil, nil
		case vault.TypeFile:
			return nil, map[string]vault.FilePayload{
				s.env: {Filename: e.Filename, Data: e.Data},
			}, nil
		default:
			return nil, nil, fmt.Errorf("%q is a group; ad-hoc binding requires a flat secret or a file secret", s.name)
		}
	}
	return v.ResolveEntry(s.name, cfg)
}

// filterOnly narrows env to the named keys. An unknown key is an error rather
// than a silent omission: a typo in a least-privilege allowlist should fail
// loudly instead of quietly running a process without the secret it needs.
func filterOnly(sel selection, only []string) (selection, error) {
	keys := splitOnly(only)
	if len(keys) == 0 {
		return sel, nil
	}

	out := selection{env: map[string]string{}, files: map[string]vault.FilePayload{}}
	var missing []string
	for _, k := range keys {
		switch val, ok := sel.env[k]; {
		case ok:
			out.env[k] = val
		default:
			p, isFile := sel.files[k]
			if !isFile {
				missing = append(missing, k)
				continue
			}
			out.files[k] = p
		}
	}
	if len(missing) > 0 {
		return selection{}, fmt.Errorf("no such key in the selection: %s (available: %s)",
			quoteList(missing), quoteList(sel.names()))
	}
	return out, nil
}

// filterExcept removes the named keys from env. Unlike --only, an unknown key is
// not an error: excluding something that is not present is a harmless no-op,
// whereas a typo in an allowlist would silently drop a secret that is needed.
func filterExcept(sel selection, except []string) selection {
	keys := splitOnly(except)
	if len(keys) == 0 {
		return sel
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	out := selection{
		env:   make(map[string]string, len(sel.env)),
		files: make(map[string]vault.FilePayload, len(sel.files)),
	}
	for k, v := range sel.env {
		if !drop[k] {
			out.env[k] = v
		}
	}
	for k, p := range sel.files {
		if !drop[k] {
			out.files[k] = p
		}
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
