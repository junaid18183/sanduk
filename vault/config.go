package vault

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the non-secret metadata store. It is deliberately safe to read,
// commit, and version: it never contains secret values, only the mapping from
// vault entries to environment variable names plus human descriptions.
type Config struct {
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

// ConfigPath resolves the config file location:
//
//	SANDUK_CONFIG            -> explicit override
//	$XDG_CONFIG_HOME/sanduk  -> XDG base dir
//	~/.config/sanduk         -> default
func ConfigPath() string {
	if p := os.Getenv("SANDUK_CONFIG"); p != "" {
		return p
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "sanduk", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "sanduk", "config.toml")
	}
	return filepath.Join(home, ".config", "sanduk", "config.toml")
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
		return nil, err
	}
	return c, nil
}

// SaveConfig writes the config file with restrictive permissions.
func SaveConfig(path string, c *Config) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	buf.WriteString("# sanduk config — metadata only. This file never contains secret values.\n\n")
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// EnvNameForSecret returns the environment variable a flat secret is exposed
// as: the configured name, or the upper-cased entry name.
func (c *Config) EnvNameForSecret(name string) string {
	if c != nil {
		if m, ok := c.Secrets[name]; ok && m.Env != "" {
			return m.Env
		}
	}
	return upper(name)
}
