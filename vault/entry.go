package vault

import (
	"encoding/json"
	"fmt"
)

// Entry types stored in the keychain.
const (
	TypeSecret = "secret"
	TypeGroup  = "group"

	entryVersion = 1
)

// Entry is the secret material for a single named vault item. It is the only
// thing ever written to the keychain; all non-secret metadata lives in the
// config file.
//
// Links are the one part of an entry that holds no secret material: they map an
// environment variable name to the name of a flat secret whose value is read at
// resolve time. They live here rather than in the metadata config so that a
// group stays self-contained in the backend — losing or moving config.toml must
// never change which variables an entry injects.
type Entry struct {
	Version int               `json:"version"`
	Type    string            `json:"type"`
	Value   string            `json:"value,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
	Links   map[string]string `json:"links,omitempty"`
}

func (e *Entry) Marshal() ([]byte, error) {
	if e.Version == 0 {
		e.Version = entryVersion
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

// UnmarshalEntry decodes and validates a stored entry.
func UnmarshalEntry(b []byte) (*Entry, error) {
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		// The decoder's message is not included: it can quote part of the
		// stored bytes, which are secret.
		return nil, fmt.Errorf("corrupt vault entry")
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	return &e, nil
}

func (e *Entry) validate() error {
	switch e.Type {
	case TypeSecret:
		if e.Value == "" {
			return fmt.Errorf("secret entry has no value")
		}
		// Only a group composes other entries. A flat secret carrying links
		// would make the link graph unbounded, which linkTargetType relies on
		// being one level deep.
		if len(e.Links) > 0 {
			return fmt.Errorf("secret entry must not have links")
		}
	case TypeGroup:
		if len(e.Fields)+len(e.Links) == 0 {
			return fmt.Errorf("group entry has no fields")
		}
		if err := validateLinks(e.Fields, e.Links); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown entry type %q", e.Type)
	}
	return nil
}

// validateLinks checks a group's links on write and again on read, so a
// hand-edited backend cannot produce a group that injects an invalid variable
// name or addresses an unusable entry.
func validateLinks(fields, links map[string]string) error {
	for env, target := range links {
		if _, dup := fields[env]; dup {
			return fmt.Errorf("%s is both a stored field and a link", env)
		}
		if err := ValidateEnvName(env); err != nil {
			return err
		}
		if err := ValidateEntryName(target); err != nil {
			return fmt.Errorf("link %s: %w", env, err)
		}
	}
	return nil
}
