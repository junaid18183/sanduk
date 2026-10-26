package vault

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/99designs/keyring"
)

// ErrNotFound is returned when an entry does not exist in the vault.
var ErrNotFound = errors.New("entry not found")

// Vault is the high-level interface to the credential store.
type Vault struct {
	kr keyring.Keyring
}

// ValidateEntryName rejects names that cannot safely address one entry. An empty
// name matters most: some backends treat a missing account as "match every
// item", so removing "" could delete the whole vault.
func ValidateEntryName(name string) error {
	if name == "" {
		return errors.New("entry name must not be empty")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("entry name %q contains a control character", name)
		}
	}
	return nil
}

// Get returns the secret entry stored under name.
func (v *Vault) Get(name string) (*Entry, error) {
	if err := ValidateEntryName(name); err != nil {
		return nil, err
	}
	item, err := v.kr.Get(name)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil, err
	}
	return UnmarshalEntry(item.Data)
}

// put writes an entry to the keychain.
func (v *Vault) put(name string, e *Entry) error {
	if err := ValidateEntryName(name); err != nil {
		return err
	}
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	return v.kr.Set(keyring.Item{
		Key:   name,
		Label: "sanduk: " + name,
		Data:  b,
	})
}

// AddSecret stores (or replaces) a flat secret.
func (v *Vault) AddSecret(name, value string) error {
	return v.put(name, &Entry{Version: entryVersion, Type: TypeSecret, Value: value})
}

// AddFile stores (or replaces) a file secret: bytes a program must read from
// a path rather than an environment variable. filename is the basename to
// give it when materialized, and may be empty.
func (v *Vault) AddFile(name string, data []byte, filename string) error {
	if len(data) == 0 {
		return fmt.Errorf("refusing to store an empty file as %q", name)
	}
	return v.put(name, &Entry{
		Version:  entryVersion,
		Type:     TypeFile,
		Data:     data,
		Filename: filename,
	})
}

// AddGroup stores (or replaces) a grouped entry. Field names become child
// environment variables, so each is validated before it is stored.
func (v *Vault) AddGroup(name string, fields map[string]string) error {
	for k := range fields {
		if err := ValidateEnvName(k); err != nil {
			return err
		}
	}
	return v.put(name, &Entry{Version: entryVersion, Type: TypeGroup, Fields: fields})
}

// MergeGroup updates individual fields of a group, leaving the fields not named
// in fields untouched. It is the "edit one field" path: the caller never has to
// re-supply values it does not want to change. A group that does not exist yet
// is created with exactly these fields, matching the upsert behaviour of add
// and add-group.
func (v *Vault) MergeGroup(name string, fields map[string]string) error {
	if len(fields) == 0 {
		return fmt.Errorf("no fields provided")
	}
	for k := range fields {
		if err := ValidateEnvName(k); err != nil {
			return err
		}
	}

	e, err := v.Get(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return v.AddGroup(name, fields)
		}
		return err
	}
	if e.Type != TypeGroup {
		return fmt.Errorf("%q is a %s, not a group", name, e.Type)
	}
	if e.Fields == nil {
		e.Fields = make(map[string]string, len(fields))
	}
	for k, val := range fields {
		e.Fields[k] = val
	}
	return v.put(name, e)
}

// UnsetGroupFields removes fields from an existing group. It refuses to leave
// the group empty, because an empty group is not a valid entry.
func (v *Vault) UnsetGroupFields(name string, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("no fields provided")
	}
	e, err := v.Get(name)
	if err != nil {
		return err
	}
	if e.Type != TypeGroup {
		return fmt.Errorf("%q is a %s, not a group", name, e.Type)
	}
	for _, k := range keys {
		if _, ok := e.Links[k]; ok {
			return fmt.Errorf("group %q: %s is a link, not a stored value; use \"sanduk group unlink %s %s\"",
				name, k, name, k)
		}
		if _, ok := e.Fields[k]; !ok {
			return fmt.Errorf("group %q has no field %q", name, k)
		}
		delete(e.Fields, k)
	}
	if len(e.Fields)+len(e.Links) == 0 {
		return fmt.Errorf("refusing to remove the last field of %q; use \"sanduk remove %s\" to delete the entry",
			name, name)
	}
	return v.put(name, e)
}

// LinkGroupField points one of a group's environment variables at a flat
// secret. Nothing is copied: the value is read from the target at resolve time,
// so rotating the target changes what the group injects without a second edit.
//
// A link may only address a flat secret. That makes a cycle impossible by
// construction rather than by a depth counter, and keeps resolution to a single
// extra read per distinct target.
func (v *Vault) LinkGroupField(group, envName, target string) error {
	if err := ValidateEnvName(envName); err != nil {
		return err
	}
	if err := ValidateEntryName(target); err != nil {
		return err
	}
	if group == target {
		return fmt.Errorf("%q cannot link to itself", group)
	}

	// Decide from the backend, never from metadata: the config can drift, and a
	// link that silently resolved to nothing would under-inject at run time.
	te, err := v.Get(target)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: %s (link target)", ErrNotFound, target)
		}
		return err
	}
	if te.Type != TypeSecret {
		return fmt.Errorf("%q is a %s; a link must point at a flat secret", target, te.Type)
	}

	e, err := v.Get(group)
	switch {
	case errors.Is(err, ErrNotFound):
		e = &Entry{Version: entryVersion, Type: TypeGroup}
	case err != nil:
		return err
	case e.Type != TypeGroup:
		return fmt.Errorf("%q is a %s, not a group", group, e.Type)
	}

	if _, ok := e.Fields[envName]; ok {
		return fmt.Errorf("%q already stores %s as a value; unset it first, or link under another name",
			group, envName)
	}
	if e.Links == nil {
		e.Links = map[string]string{}
	}
	e.Links[envName] = target
	return v.put(group, e)
}

// UnlinkGroupField removes a link, leaving the target secret untouched.
func (v *Vault) UnlinkGroupField(group string, envNames []string) error {
	if len(envNames) == 0 {
		return fmt.Errorf("no fields provided")
	}
	e, err := v.Get(group)
	if err != nil {
		return err
	}
	if e.Type != TypeGroup {
		return fmt.Errorf("%q is a %s, not a group", group, e.Type)
	}
	for _, k := range envNames {
		if _, ok := e.Links[k]; !ok {
			if _, stored := e.Fields[k]; stored {
				return fmt.Errorf("group %q: %s is a stored value, not a link; use \"sanduk group unset %s %s\"",
					group, k, group, k)
			}
			return fmt.Errorf("group %q has no link %q", group, k)
		}
		delete(e.Links, k)
	}
	if len(e.Fields)+len(e.Links) == 0 {
		return fmt.Errorf("refusing to remove the last field of %q; use \"sanduk remove %s\" to delete the entry",
			group, group)
	}
	return v.put(group, e)
}

// resolveLinks expands a group's links into out. Targets are read once each, so
// a group that links the same secret under two names costs one read — which on
// a presence-gated backend is one prompt. Names are walked in sorted order so a
// dangling link reports the same field every time.
func (v *Vault) resolveLinks(group string, e *Entry, out map[string]string) error {
	if len(e.Links) == 0 {
		return nil
	}
	envNames := make([]string, 0, len(e.Links))
	for env := range e.Links {
		envNames = append(envNames, env)
	}
	sort.Strings(envNames)

	cache := make(map[string]string, len(e.Links))
	for _, env := range envNames {
		target := e.Links[env]
		val, ok := cache[target]
		if !ok {
			var err error
			if val, err = v.linkValue(group, env, target); err != nil {
				return err
			}
			cache[target] = val
		}
		out[env] = val
	}
	return nil
}

// linkValue reads one link target, failing closed with a message that names the
// group and field rather than leaving the variable quietly absent.
func (v *Vault) linkValue(group, env, target string) (string, error) {
	te, err := v.Get(target)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("group %q: field %s links to %q, which is not in the vault",
				group, env, target)
		}
		return "", err
	}
	if te.Type != TypeSecret {
		return "", fmt.Errorf("group %q: field %s links to %q, which is a %s, not a flat secret",
			group, env, target, te.Type)
	}
	return te.Value, nil
}

// GroupValues returns every field of a group with links resolved. It is the
// read path for printing a whole group; ResolveEnv is the one for injection.
func (v *Vault) GroupValues(name string) (map[string]string, error) {
	e, err := v.Get(name)
	if err != nil {
		return nil, err
	}
	if e.Type != TypeGroup {
		return nil, fmt.Errorf("%q is a %s, not a group", name, e.Type)
	}
	out := make(map[string]string, len(e.Fields)+len(e.Links))
	for k, val := range e.Fields {
		out[k] = val
	}
	if err := v.resolveLinks(name, e, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GroupFieldValue returns one field of a group, following a link if that field
// is one, and reports the entry the value came from (empty when it is stored in
// the group itself). Only the named field is resolved, so reviewing one secret
// never reads the rest of the group.
func (v *Vault) GroupFieldValue(name, key string) (val, linkedTo string, err error) {
	e, err := v.Get(name)
	if err != nil {
		return "", "", err
	}
	if e.Type != TypeGroup {
		return "", "", fmt.Errorf("%q is a %s, not a group", name, e.Type)
	}
	if target, ok := e.Links[key]; ok {
		val, err := v.linkValue(name, key, target)
		return val, target, err
	}
	if val, ok := e.Fields[key]; ok {
		return val, "", nil
	}
	return "", "", fmt.Errorf("%q has no field %q (fields: %s)",
		name, key, strings.Join(groupFieldNames(e), ", "))
}

// GroupFieldNames returns a group's field names, sorted. The values are read as
// part of loading the entry but are never returned or displayed.
func (v *Vault) GroupFieldNames(name string) ([]string, error) {
	e, err := v.Get(name)
	if err != nil {
		return nil, err
	}
	if e.Type != TypeGroup {
		return nil, fmt.Errorf("%q is a %s, not a group", name, e.Type)
	}
	return groupFieldNames(e), nil
}

// groupFieldNames returns every variable name a group contributes, stored and
// linked alike, sorted.
func groupFieldNames(e *Entry) []string {
	names := make([]string, 0, len(e.Fields)+len(e.Links))
	for k := range e.Fields {
		names = append(names, k)
	}
	for k := range e.Links {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Variables describes what one entry contributes to a child's environment: the
// variable names, and for a linked field the flat secret its value is read
// from. It never carries a value, so it is safe to print.
type Variables struct {
	Type  string
	Names []string
	// Links maps a variable name to the flat secret it resolves through.
	// Absent for a variable the entry stores itself.
	Links map[string]string
}

// Variables reads one entry and reports the variables it provides. It is the
// single read behind a project-scoped listing: one entry, names only, so the
// cost is one decrypt (and on a presence-gated backend one prompt), not a walk
// of the whole vault.
func (v *Vault) Variables(name string, c *Config) (*Variables, error) {
	e, err := v.Get(name)
	if err != nil {
		return nil, err
	}
	out := &Variables{Type: e.Type}
	if e.Type == TypeGroup {
		out.Names, out.Links = groupFieldNames(e), e.Links
		return out, nil
	}
	// A flat or file secret contributes the one variable it is mapped to.
	out.Names = []string{c.EnvNameForSecret(name)}
	return out, nil
}

// Remove deletes an entry.
func (v *Vault) Remove(name string) error {
	if err := ValidateEntryName(name); err != nil {
		return err
	}
	if err := v.kr.Remove(name); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return err
	}
	return nil
}

// Exists reports whether an entry named name is stored. It lists names rather
// than reading the entry, so it never decrypts a value or triggers a
// user-presence prompt (not every backend supports metadata-only reads).
func (v *Vault) Exists(name string) (bool, error) {
	if err := ValidateEntryName(name); err != nil {
		return false, err
	}
	names, err := v.kr.Keys()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

// List returns all entry names, sorted.
func (v *Vault) List() ([]string, error) {
	keys, err := v.kr.Keys()
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// ResolveEnv expands a named entry into environment variables. It is the
// variables-only view: a file secret has no value until it is materialized, so
// an entry that needs one is refused here. Callers that can materialize use
// ResolveEntry.
func (v *Vault) ResolveEnv(name string, cfg *Config) (map[string]string, error) {
	env, files, err := v.ResolveEntry(name, cfg)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		return nil, fmt.Errorf("%q is a file secret; it has no value to print, only a path "+
			"(use \"sanduk exec\", or \"sanduk export --materialize DIR\")", name)
	}
	return env, nil
}

// ResolveEntry expands a named entry into environment variables plus any file
// secrets the caller must materialize, keyed by the variable that will hold
// each one's path.
//
//   - A flat secret becomes a single variable, named by the config mapping or
//     the upper-cased entry name.
//   - A group contributes one variable per field; field names are env names.
//   - A file secret contributes one payload under its mapped variable name.
func (v *Vault) ResolveEntry(name string, cfg *Config) (map[string]string, map[string]FilePayload, error) {
	e, err := v.Get(name)
	if err != nil {
		return nil, nil, err
	}
	switch e.Type {
	case TypeSecret:
		return map[string]string{cfg.EnvNameForSecret(name): e.Value}, nil, nil
	case TypeFile:
		return nil, map[string]FilePayload{
			cfg.EnvNameForSecret(name): {Filename: e.Filename, Data: e.Data},
		}, nil
	case TypeGroup:
		out := make(map[string]string, len(e.Fields)+len(e.Links))
		for k, val := range e.Fields {
			out[k] = val
		}
		if err := v.resolveLinks(name, e, out); err != nil {
			return nil, nil, err
		}
		return out, nil, nil
	default:
		return nil, nil, fmt.Errorf("unknown entry type %q", e.Type)
	}
}
