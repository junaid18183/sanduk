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

// New opens the vault using the configured backend.
func New() (*Vault, error) {
	kr, err := OpenKeyring()
	if err != nil {
		return nil, err
	}
	return &Vault{kr: kr}, nil
}

// Get returns the secret entry stored under name.
func (v *Vault) Get(name string) (*Entry, error) {
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

// AddGroup stores (or replaces) a grouped entry.
func (v *Vault) AddGroup(name string, fields map[string]string) error {
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
		if k == "" {
			return fmt.Errorf("empty field name")
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
	if err := v.kr.Remove(name); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return err
	}
	return nil
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

func upper(s string) string {
	return strings.ToUpper(s)
}
