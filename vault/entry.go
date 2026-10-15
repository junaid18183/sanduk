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
type Entry struct {
	Version int               `json:"version"`
	Type    string            `json:"type"`
	Value   string            `json:"value,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
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
		return nil, fmt.Errorf("corrupt vault entry: %w", err)
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
	case TypeGroup:
		if len(e.Fields) == 0 {
			return fmt.Errorf("group entry has no fields")
		}
	default:
		return fmt.Errorf("unknown entry type %q", e.Type)
	}
	return nil
}
