package vault

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the non-secret metadata store. It is deliberately safe to read,
// commit, and version: it never contains secret values, only the mapping from
// vault entries to environment variable names plus human descriptions.
type Config struct {
	// Backend settings. These are defaults only: a --flag or an environment
	// variable always wins, so one config file can serve several machines with
	// different keystores. All three are optional.
	Backend  string `toml:"backend,omitempty"`
	Keychain string `toml:"keychain,omitempty"`
	Service  string `toml:"service,omitempty"`

	Secrets map[string]SecretMeta `toml:"secrets,omitempty"`
	Groups  map[string]GroupMeta  `toml:"groups,omitempty"`
	// Files describes file secrets. They are kept in their own table rather
	// than mixed in with Secrets so list can name the kind without reading
	// anything from the backend.
	Files map[string]SecretMeta `toml:"files,omitempty"`
}

// SecretMeta describes a flat secret entry.
type SecretMeta struct {
	Env         string `toml:"env,omitempty"`
	Description string `toml:"description,omitempty"`
	// Expires is a YYYY-MM-DD date after which the credential should be
	// rotated. It is a reminder, not an enforcement: sanduk never refuses to
	// inject an expired secret, because failing a deploy over a date in a
	// metadata file would be worse than the stale key it is warning about.
	Expires string `toml:"expires,omitempty"`
}

// GroupMeta describes a grouped entry. Group field names are the environment
// variable names, so no explicit mapping is required.
type GroupMeta struct {
	Description string `toml:"description,omitempty"`
	Expires     string `toml:"expires,omitempty"`
}

// LoadConfig reads the config file, returning an empty config if it does not
// exist yet.
func LoadConfig(path string) (*Config, error) {
	c := &Config{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// UpdateConfig loads the config at path, applies fn and saves the result, all
// under a cross-process lock. Without the lock two concurrent commands (two
// `sanduk add` in parallel scripts, say) each load, change and save the file,
// and one of the updates is lost.
func UpdateConfig(path string, fn func(*Config) error) error {
	lk, err := lockPath(path)
	if err != nil {
		return err
	}
	defer lk.release()
	c, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	return SaveConfig(path, c)
}

// RecordSecret marks name as a flat secret, clearing any group record. An
// empty env or desc leaves the existing value alone.
func (c *Config) RecordSecret(name, env, desc string) {
	if c.Secrets == nil {
		c.Secrets = map[string]SecretMeta{}
	}
	m := c.Secrets[name]
	if env != "" {
		m.Env = env
	}
	if desc != "" {
		m.Description = desc
	}
	c.Secrets[name] = m
	delete(c.Groups, name)
	delete(c.Files, name)
}

// RecordFile marks name as a file secret, clearing any other record.
func (c *Config) RecordFile(name, env, desc string) {
	if c.Files == nil {
		c.Files = map[string]SecretMeta{}
	}
	m := c.Files[name]
	if env != "" {
		m.Env = env
	}
	if desc != "" {
		m.Description = desc
	}
	c.Files[name] = m
	delete(c.Secrets, name)
	delete(c.Groups, name)
}

// RecordGroup marks name as a group, clearing any secret record. An empty desc
// leaves the existing description alone.
func (c *Config) RecordGroup(name, desc string) {
	if c.Groups == nil {
		c.Groups = map[string]GroupMeta{}
	}
	g := c.Groups[name]
	if desc != "" {
		g.Description = desc
	}
	c.Groups[name] = g
	delete(c.Secrets, name)
	delete(c.Files, name)
}

// ExpiryLayout is the date format used by the expires field. A plain date,
// not a timestamp: rotation reminders are a day-granularity concern, and a
// date is what a provider's dashboard shows.
const ExpiryLayout = "2006-01-02"

// ParseExpiry validates a YYYY-MM-DD expiry date.
func ParseExpiry(s string) (time.Time, error) {
	t, err := time.Parse(ExpiryLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiry %q is not a YYYY-MM-DD date", s)
	}
	return t, nil
}

// SetExpiry records (or with an empty date clears) an entry's expiry, in
// whichever metadata map already holds it. It reports whether the entry had a
// record to update: an entry with no metadata cannot carry an expiry, and
// silently creating one would guess at its type.
func (c *Config) SetExpiry(name, date string) bool {
	if m, ok := c.Secrets[name]; ok {
		m.Expires = date
		c.Secrets[name] = m
		return true
	}
	if m, ok := c.Files[name]; ok {
		m.Expires = date
		c.Files[name] = m
		return true
	}
	if g, ok := c.Groups[name]; ok {
		g.Expires = date
		c.Groups[name] = g
		return true
	}
	return false
}

// ExpiryOf returns the recorded expiry date for an entry, or "".
func (c *Config) ExpiryOf(name string) string {
	if c == nil {
		return ""
	}
	if m, ok := c.Secrets[name]; ok {
		return m.Expires
	}
	if m, ok := c.Files[name]; ok {
		return m.Expires
	}
	if g, ok := c.Groups[name]; ok {
		return g.Expires
	}
	return ""
}

// Expiry is one entry's rotation deadline, as doctor reports it.
type Expiry struct {
	Name string
	Date string
	Days int // whole days from now until Date; negative once past
}

// Expired reports whether the deadline has passed.
func (e Expiry) Expired() bool { return e.Days < 0 }

// Expiries returns every entry carrying an expiry date, sorted by how soon it
// falls due. Entries whose date does not parse are returned last with a zero
// day count, so a hand-edited config surfaces rather than being skipped.
// It reads only metadata: no value is ever decrypted to answer this.
func (c *Config) Expiries(now time.Time) ([]Expiry, error) {
	dates := map[string]string{}
	for name, m := range c.Secrets {
		if m.Expires != "" {
			dates[name] = m.Expires
		}
	}
	for name, m := range c.Files {
		if m.Expires != "" {
			dates[name] = m.Expires
		}
	}
	for name, g := range c.Groups {
		if g.Expires != "" {
			dates[name] = g.Expires
		}
	}

	today := now.UTC().Truncate(24 * time.Hour)
	out := make([]Expiry, 0, len(dates))
	for _, name := range sortedKeys(dates) {
		t, err := ParseExpiry(dates[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, Expiry{
			Name: name,
			Date: dates[name],
			Days: int(t.Sub(today).Hours() / 24),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Days < out[j].Days })
	return out, nil
}

// sortedKeys returns a map's keys in order, so output is deterministic.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Forget removes every metadata record for name.
func (c *Config) Forget(name string) {
	delete(c.Secrets, name)
	delete(c.Groups, name)
	delete(c.Files, name)
}

// SaveConfig writes the config file with restrictive permissions. The write is
// atomic, so a crash mid-save cannot truncate the metadata of every entry, and
// two concurrent writers cannot leave a half-written file behind.
func SaveConfig(path string, c *Config) error {
	var buf bytes.Buffer
	buf.WriteString("# sanduk config — metadata only. This file never contains secret values.\n\n")
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

// EnvNameForSecret returns the environment variable a flat secret is exposed
// as: the configured name, or the upper-cased entry name.
func (c *Config) EnvNameForSecret(name string) string {
	if c != nil {
		if m, ok := c.Secrets[name]; ok && m.Env != "" {
			return m.Env
		}
		if m, ok := c.Files[name]; ok && m.Env != "" {
			return m.Env
		}
	}
	return strings.ToUpper(name)
}

// MetadataDrift compares the entry names a backend holds with the config's
// metadata. untracked entries exist in the backend with no metadata (list then
// guesses their type); orphaned records describe entries the backend no longer
// has. Both results are sorted.
func MetadataDrift(keys []string, c *Config) (untracked, orphaned []string) {
	inBackend := make(map[string]bool, len(keys))
	for _, k := range keys {
		inBackend[k] = true
		_, s := c.Secrets[k]
		_, g := c.Groups[k]
		_, f := c.Files[k]
		if !s && !g && !f {
			untracked = append(untracked, k)
		}
	}
	for name := range c.Secrets {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	for name := range c.Groups {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	for name := range c.Files {
		if !inBackend[name] {
			orphaned = append(orphaned, name)
		}
	}
	sort.Strings(untracked)
	sort.Strings(orphaned)
	return untracked, orphaned
}
