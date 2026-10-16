package vault

import (
	"errors"
	"io"
	"os"
)

// Options selects and configures the keystore a Vault opens. The vault package
// never reads the environment or prompts: the CLI resolves every setting
// (flag > environment > config > default) and passes it in here.
//
// Empty Backend, Keychain and Service mean the platform keystore, the "login"
// keychain and the "sanduk" service.
type Options struct {
	Backend  string
	Keychain string
	Service  string

	// FileDir is the age backend's directory. It is required whenever the age
	// backend is used, including as the fallback when no native keystore is
	// compiled in.
	FileDir string

	// PassFunc supplies the age backend's passphrase. It is required whenever
	// the age backend is used.
	PassFunc func(prompt string) (string, error)

	// KeychainPassphrase is used only when a dedicated macOS keychain has to be
	// created. Empty means the system prompts for one.
	KeychainPassphrase string

	// Warn receives non-fatal notices. Nil means os.Stderr.
	Warn io.Writer
}

func (o Options) warn() io.Writer {
	if o.Warn != nil {
		return o.Warn
	}
	return os.Stderr
}

func (o Options) passFunc() func(string) (string, error) {
	if o.PassFunc != nil {
		return o.PassFunc
	}
	return func(string) (string, error) {
		return "", errors.New("no passphrase source for the age backend")
	}
}

// Open opens the vault described by o.
func Open(o Options) (*Vault, error) {
	kr, err := openKeyring(o)
	if err != nil {
		return nil, err
	}
	return &Vault{kr: kr}, nil
}
