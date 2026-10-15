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
