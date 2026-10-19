package vault

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	UseFastScryptForTests()
	os.Exit(m.Run())
}

// testOptions opens the age backend in a fresh temporary directory with a fake
// passphrase. The vault package reads no environment, so this is the only way a
// test reaches a backend: never the developer's real keychain.
func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Backend:  "file",
		Service:  "sanduk-test",
		FileDir:  t.TempDir(),
		PassFunc: func(string) (string, error) { return "test-passphrase", nil },
	}
}
