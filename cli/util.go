package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
)

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

// parseJSONObject decodes a JSON object whose values are all strings.
func parseJSONObject(s string) (map[string]string, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		// Not wrapped: a JSON syntax error can quote part of the input.
		return nil, fmt.Errorf("invalid JSON object")
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

// readDotenvFile parses a .env file, naming the file in any error.
func readDotenvFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := vault.ParseDotenv(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// shortPath renders a path for display, with the home directory as "~". The
// paths in a listing are long and mostly identical prefix; what the reader is
// looking for is the part that differs.
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
