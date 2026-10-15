package vault

import (
	"path/filepath"
	"testing"
)

func TestConfigPathPrecedence(t *testing.T) {
	t.Setenv("SANDUK_CONFIG", "/tmp/explicit.toml")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := ConfigPath(); got != "/tmp/explicit.toml" {
		t.Fatalf("SANDUK_CONFIG should win, got %q", got)
	}

	t.Setenv("SANDUK_CONFIG", "")
	if got := ConfigPath(); got != filepath.Join("/tmp/xdg", "sanduk", "config.toml") {
		t.Fatalf("XDG should apply, got %q", got)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")

	c := &Config{
		Secrets: map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY", Description: "key"}},
		Groups:  map[string]GroupMeta{"aws-prod": {Description: "production"}},
	}
	if err := SaveConfig(path, c); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Secrets["openai"].Env != "OPENAI_API_KEY" {
		t.Fatalf("secret meta lost: %+v", got.Secrets)
	}
	if got.Groups["aws-prod"].Description != "production" {
		t.Fatalf("group meta lost: %+v", got.Groups)
	}
}

func TestLoadConfigMissingIsEmpty(t *testing.T) {
	got, err := LoadConfig(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(got.Secrets) != 0 || len(got.Groups) != 0 {
		t.Fatalf("expected empty config, got %+v", got)
	}
}

func TestEnvNameForSecret(t *testing.T) {
	c := &Config{Secrets: map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY"}}}
	if got := c.EnvNameForSecret("openai"); got != "OPENAI_API_KEY" {
		t.Fatalf("mapped env = %q", got)
	}
	// Unmapped names default to the upper-cased entry name.
	if got := c.EnvNameForSecret("deepseek"); got != "DEEPSEEK" {
		t.Fatalf("default env = %q", got)
	}
	// A nil config must still work.
	var nilCfg *Config
	if got := nilCfg.EnvNameForSecret("k"); got != "K" {
		t.Fatalf("nil config env = %q", got)
	}
}
