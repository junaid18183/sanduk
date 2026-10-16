package vault

import (
	"errors"
	"fmt"
	"sort"

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
		if _, ok := e.Fields[k]; !ok {
			return fmt.Errorf("group %q has no field %q", name, k)
		}
		delete(e.Fields, k)
	}
	if len(e.Fields) == 0 {
		return fmt.Errorf("refusing to remove the last field of %q; use \"sanduk remove %s\" to delete the entry",
			name, name)
	}
	return v.put(name, e)
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
	names := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		names = append(names, k)
	}
	sort.Strings(names)
	return names, nil
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

// ResolveEnv expands a named entry into environment variables.
//
//   - A flat secret becomes a single variable, named by the config mapping or
//     the upper-cased entry name.
//   - A group contributes one variable per field; field names are env names.
func (v *Vault) ResolveEnv(name string, cfg *Config) (map[string]string, error) {
	e, err := v.Get(name)
	if err != nil {
		return nil, err
	}
	switch e.Type {
	case TypeSecret:
		return map[string]string{cfg.EnvNameForSecret(name): e.Value}, nil
	case TypeGroup:
		out := make(map[string]string, len(e.Fields))
		for k, val := range e.Fields {
			out[k] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown entry type %q", e.Type)
	}
}
