package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// TestMain points every SANDUK_* setting at a throwaway age vault before any
// test runs, so a command executed by a test can never reach the developer's
// real keychain or config, even if the test forgets testEnv.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sanduk-cli-test-")
	if err != nil {
		panic(err)
	}
	for k, v := range map[string]string{
		"SANDUK_BACKEND":         "file",
		"SANDUK_FILE_DIR":        filepath.Join(dir, "data"),
		"SANDUK_FILE_PASSPHRASE": "test-passphrase",
		"SANDUK_CONFIG":          filepath.Join(dir, "config", "config.toml"),
		"SANDUK_SERVICE":         "sanduk-cli-test",
		"SANDUK_KEYCHAIN":        "",
		"SANDUK_TRUST":           "",
	} {
		os.Setenv(k, v)
	}
	vault.UseFastScryptForTests()
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
