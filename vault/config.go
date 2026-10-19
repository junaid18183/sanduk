package vault

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

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
}

// SecretMeta describes a flat secret entry.
type SecretMeta struct {
	Env         string `toml:"env,omitempty"`
	Description string `toml:"description,omitempty"`
}

// GroupMeta describes a grouped entry. Group field names are the environment
// variable names, so no explicit mapping is required.
type GroupMeta struct {
	Description string `toml:"description,omitempty"`
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
}

// Forget removes every metadata record for name.
func (c *Config) Forget(name string) {
	delete(c.Secrets, name)
	delete(c.Groups, name)
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
		if !s && !g {
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
	sort.Strings(untracked)
	sort.Strings(orphaned)
	return untracked, orphaned
}
