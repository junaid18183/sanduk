package vault

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Entry types stored in the keychain.
const (
	TypeSecret = "secret"
	TypeGroup  = "group"
	TypeFile   = "file"

	entryVersion = 1
)

// Entry is the secret material for a single named vault item. It is the only
// thing ever written to the keychain; all non-secret metadata lives in the
// config file.
//
// Links are the one part of an entry that holds no secret material: they map an
// environment variable name to the name of a flat secret whose value is read at
// resolve time. FieldLinks do the same for a stored field of another group,
// written "Group/FIELD". They live here rather than in the metadata config so
// that a group stays self-contained in the backend — losing or moving
// config.toml must never change which variables an entry injects.
type Entry struct {
	Version    int               `json:"version"`
	Type       string            `json:"type"`
	Value      string            `json:"value,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	Links      map[string]string `json:"links,omitempty"`
	FieldLinks map[string]string `json:"field_links,omitempty"`

	// Data is a file secret's contents, and Filename the basename it should
	// be given when materialized (some tools key off the extension). Data is
	// base64-encoded by encoding/json, so a credential JSON, a .p8 key or a
	// certificate all round-trip unchanged.
	Data     []byte `json:"data,omitempty"`
	Filename string `json:"filename,omitempty"`
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
		if len(e.Data) > 0 {
			return fmt.Errorf("secret entry must not carry file data")
		}
		// Only a group composes other entries. A flat secret carrying links
		// would make the link graph unbounded, which linkTargetType relies on
		// being one level deep.
		if len(e.Links) > 0 || len(e.FieldLinks) > 0 {
			return fmt.Errorf("secret entry must not have links")
		}
	case TypeGroup:
		// A group may be empty: add-group creates one before any field is
		// set, and group add fills it in later.
		if err := validateLinks(e); err != nil {
			return err
		}
		if len(e.Data) > 0 {
			return fmt.Errorf("group entry must not carry file data")
		}
	case TypeFile:
		if len(e.Data) == 0 {
			return fmt.Errorf("file entry has no data")
		}
		// A file secret is one opaque blob. Letting it also carry fields or
		// links would give it two resolution paths, and only one of them can
		// be the variable the child receives.
		if e.Value != "" || len(e.Fields) > 0 || len(e.Links) > 0 || len(e.FieldLinks) > 0 {
			return fmt.Errorf("file entry must carry only data")
		}
	default:
		return fmt.Errorf("unknown entry type %q", e.Type)
	}
	return nil
}

// validateLinks checks a group's links on write and again on read, so a
// hand-edited backend cannot produce a group that injects an invalid variable
// name or addresses an unusable entry.
func validateLinks(e *Entry) error {
	for env := range e.Fields {
		if _, dup := e.Links[env]; dup {
			return fmt.Errorf("%s is both a stored field and a link", env)
		}
		if _, dup := e.FieldLinks[env]; dup {
			return fmt.Errorf("%s is both a stored field and a link", env)
		}
	}
	for env := range e.Links {
		if _, dup := e.FieldLinks[env]; dup {
			return fmt.Errorf("%s is linked twice", env)
		}
	}
	for env, target := range e.Links {
		if err := ValidateEnvName(env); err != nil {
			return err
		}
		if err := ValidateEntryName(target); err != nil {
			return fmt.Errorf("link %s: %w", env, err)
		}
	}
	for env, ref := range e.FieldLinks {
		if err := ValidateEnvName(env); err != nil {
			return err
		}
		if _, _, ok := SplitFieldRef(ref); !ok {
			return fmt.Errorf("link %s: %q is not Group/FIELD", env, ref)
		}
	}
	return nil
}

// FieldCount is the number of variables a group contributes, stored and linked
// alike.
func (e *Entry) FieldCount() int {
	return len(e.Fields) + len(e.Links) + len(e.FieldLinks)
}

// IsFieldRef reports whether a link target names a group's field rather than a
// flat secret. Only the field form contains "/": a flat secret's name may
// legally contain one, so a target written with it is read as Group/FIELD.
func IsFieldRef(target string) bool {
	return strings.Contains(target, "/")
}

// SplitFieldRef splits "Group/FIELD" at the last "/". A variable name never
// contains one, so the field is always the part after it, while the group may
// itself contain "/".
func SplitFieldRef(ref string) (group, field string, ok bool) {
	i := strings.LastIndex(ref, "/")
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	return ref[:i], ref[i+1:], true
}
