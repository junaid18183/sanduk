package vault

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTripsBackendSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	in := &Config{
		Backend:  "age",
		Keychain: "sanduk",
		Service:  "work",
		Secrets:  map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY"}},
	}
	if err := SaveConfig(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Backend != "age" || out.Keychain != "sanduk" || out.Service != "work" {
		t.Fatalf("settings did not round-trip: %+v", out)
	}
	if out.Secrets["openai"].Env != "OPENAI_API_KEY" {
		t.Fatalf("secrets did not round-trip: %+v", out.Secrets)
	}
}

// A config written by an older sanduk has no settings section, and must still
// load with them unset rather than erroring.
func TestConfigWithoutSettingsLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeTestFile(t, path, "[secrets]\n  [secrets.openai]\n    env = \"OPENAI_API_KEY\"\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Backend != "" || cfg.Keychain != "" || cfg.Service != "" {
		t.Fatalf("expected unset settings, got %+v", cfg)
	}
	if cfg.EnvNameForSecret("openai") != "OPENAI_API_KEY" {
		t.Fatalf("secrets were not parsed: %+v", cfg.Secrets)
	}
}
