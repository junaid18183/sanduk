package vault

import (
	"errors"
	"strings"
	"testing"
)

// newFileVault opens a vault against the encrypted file backend so tests never
// touch the developer's real keychain.
func newFileVault(t *testing.T) *Vault {
	t.Helper()
	v, err := Open(testOptions(t))
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	return v
}

func TestVaultSecretLifecycle(t *testing.T) {
	v := newFileVault(t)

	if err := v.AddSecret("openai", "sk-test-value"); err != nil {
		t.Fatalf("add: %v", err)
	}

	e, err := v.Get("openai")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if e.Type != TypeSecret || e.Value != "sk-test-value" {
		t.Fatalf("unexpected entry: %+v", e)
	}

	names, err := v.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 1 || names[0] != "openai" {
		t.Fatalf("unexpected names: %v", names)
	}

	if err := v.Remove("openai"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := v.Get("openai"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after remove, got %v", err)
	}
}

func TestResolveEnvSecretUsesConfigMapping(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("openai", "sk-test-value"); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{Secrets: map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY"}}}
	env, err := v.ResolveEnv("openai", cfg)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if env["OPENAI_API_KEY"] != "sk-test-value" {
		t.Fatalf("unexpected env: %v", env)
	}
}

func TestResolveEnvSecretDefaultsToUpperName(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("deepseek", "ds-value"); err != nil {
		t.Fatal(err)
	}
	env, err := v.ResolveEnv("deepseek", &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if env["DEEPSEEK"] != "ds-value" {
		t.Fatalf("unexpected env: %v", env)
	}
}

func TestResolveEnvGroup(t *testing.T) {
	v := newFileVault(t)
	fields := map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIA-test",
		"AWS_SECRET_ACCESS_KEY": "secret-test",
	}
	if err := v.AddGroup("aws-prod", fields); err != nil {
		t.Fatal(err)
	}

	env, err := v.ResolveEnv("aws-prod", &Config{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(env) != 2 || env["AWS_ACCESS_KEY_ID"] != "AKIA-test" {
		t.Fatalf("unexpected env: %v", env)
	}
}

func TestResolveEnvMissing(t *testing.T) {
	v := newFileVault(t)
	if _, err := v.ResolveEnv("nope", &Config{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBuildEnvOverrides(t *testing.T) {
	t.Setenv("SANDUK_TEST_BASE", "base")
	out := buildEnv(map[string]string{"SANDUK_TEST_BASE": "override", "SANDUK_TEST_NEW": "new"})

	seen := map[string]int{}
	for _, kv := range out {
		k, v, _ := strings.Cut(kv, "=")
		seen[k]++
		if k == "SANDUK_TEST_BASE" && v != "override" {
			t.Fatalf("BASE = %q, want override", v)
		}
		if k == "SANDUK_TEST_NEW" && v != "new" {
			t.Fatalf("NEW = %q, want new", v)
		}
	}
	if seen["SANDUK_TEST_BASE"] != 1 {
		t.Fatalf("BASE appears %d times, want 1", seen["SANDUK_TEST_BASE"])
	}
}
