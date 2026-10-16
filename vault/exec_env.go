package vault

import (
	"os"
	"sort"
	"strings"
)

// sandukEnvPrefix marks sanduk's own configuration variables. They configure the
// sanduk process, not the command being injected, and must never be handed to a
// child: SANDUK_FILE_PASSPHRASE and SANDUK_KEYCHAIN_PASSPHRASE each unlock the
// whole vault, so leaking either would defeat --only least privilege.
const sandukEnvPrefix = "SANDUK_"

// buildEnv returns the process environment with the given overrides applied.
//
// sanduk's own SANDUK_* variables are dropped from the inherited environment so
// a child process cannot read the vault passphrase (or any other sanduk setting)
// out of its own environment. Overrides may still set any name, including a
// SANDUK_* one, since an override is an explicit injection. Keys are
// de-duplicated, and the output order is stable across runs.
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
		if strings.HasPrefix(k, sandukEnvPrefix) {
			continue
		}
		add(k, v)
	}

	// Sort the overrides so the injected block is deterministic; the parent
	// environment already arrives in a stable order from os.Environ.
	names := make([]string, 0, len(overrides))
	for k := range overrides {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		add(k, overrides[k])
	}

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+vals[k])
	}
	return out
}
