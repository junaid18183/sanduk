package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
)

// warnf writes a status message to stderr so stdout stays machine-readable.
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func cut(s string, sep byte) (before, after string, found bool) {
	i := strings.IndexByte(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

func errInvalidField(f string) error {
	return fmt.Errorf("invalid --field %q, want KEY=VALUE", f)
}

func errNoFields() error {
	return fmt.Errorf("no fields provided; use --field KEY=VALUE, --from-json or --from-dotenv")
}

// dedupeStrings removes duplicates, preserving first-seen order.
func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// sortedMapKeys returns a map's keys, sorted.
func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parseJSONObject(s string) (map[string]string, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON object: %w", err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("field %q must be a string", k)
		}
		out[k] = str
	}
	return out, nil
}

// recordSecretMeta marks name as a flat secret in the metadata config and
// clears any stale group record. This keeps list accurate without reading
// secret values from the keychain.
func recordSecretMeta(name, env, desc string) error {
	return updateConfig(func(c *vault.Config) {
		if c.Secrets == nil {
			c.Secrets = map[string]vault.SecretMeta{}
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
	})
}

// recordGroupMeta marks name as a group in the metadata config and clears any
// stale secret record.
func recordGroupMeta(name, desc string) error {
	return updateConfig(func(c *vault.Config) {
		if c.Groups == nil {
			c.Groups = map[string]vault.GroupMeta{}
		}
		g := c.Groups[name]
		if desc != "" {
			g.Description = desc
		}
		c.Groups[name] = g
		delete(c.Secrets, name)
	})
}

func readDotenvFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return vault.ParseDotenv(b)
}
