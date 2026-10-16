package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points every SANDUK_* setting at a throwaway age vault before any
// test runs. A test that forgets newFileVault therefore opens a temporary file,
// never the developer's real keychain or config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sanduk-vault-test-")
	if err != nil {
		panic(err)
	}
	isolateForTests(dir)
	UseFastScryptForTests()
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func isolateForTests(dir string) {
	for k, v := range map[string]string{
		"SANDUK_BACKEND":         "file",
		"SANDUK_FILE_DIR":        filepath.Join(dir, "data"),
		"SANDUK_FILE_PASSPHRASE": "test-passphrase",
		"SANDUK_CONFIG":          filepath.Join(dir, "config", "config.toml"),
		"SANDUK_SERVICE":         "sanduk-test",
		"SANDUK_KEYCHAIN":        "",
		"SANDUK_TRUST":           "",
	} {
		os.Setenv(k, v)
	}
}
