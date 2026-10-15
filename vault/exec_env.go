package vault

import (
	"os"
	"strings"
)

// buildEnv returns the process environment with the given overrides applied,
// de-duplicating keys and preserving a stable order.
func buildEnv(overrides map[string]string) []string {
	seen := make(map[string]int)
	var keys []string
	vals := make(map[string]string)

	add := func(k, v string) {
		if _, ok := seen[k]; !ok {
			seen[k] = len(keys)
			keys = append(keys, k)
		}
		vals[k] = v
	}

	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		add(k, v)
	}
	for k, v := range overrides {
		add(k, v)
	}

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+vals[k])
	}
	return out
}
