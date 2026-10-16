package vault

import (
	"io"
	"os"
)

// Options selects and configures the keystore a Vault opens. The CLI resolves
// them once (flag > environment > config > default) and passes them in, so
// nothing has to travel through process-wide environment variables.
//
// Empty fields mean "default": the platform keystore, the "login" keychain and
// the "sanduk" service.
type Options struct {
	Backend  string
	Keychain string
	Service  string

	// FileDir is the age backend's directory. Empty means SANDUK_FILE_DIR, then
	// $XDG_DATA_HOME/sanduk, then ~/.local/share/sanduk.
	FileDir string

	// PassFunc supplies the age backend's passphrase. Nil means
	// SANDUK_FILE_PASSPHRASE, then an echo-off terminal prompt.
	PassFunc func(prompt string) (string, error)

	// KeychainPassphrase is used only when a dedicated macOS keychain has to be
	// created. Empty means a prompt.
	KeychainPassphrase string

	// Warn receives non-fatal notices. Nil means os.Stderr.
	Warn io.Writer
}

// OptionsFromEnv reads Options from the SANDUK_* environment variables.
func OptionsFromEnv() Options {
	return Options{
		Backend:            os.Getenv("SANDUK_BACKEND"),
		Keychain:           os.Getenv("SANDUK_KEYCHAIN"),
		Service:            os.Getenv("SANDUK_SERVICE"),
		FileDir:            os.Getenv("SANDUK_FILE_DIR"),
		KeychainPassphrase: os.Getenv("SANDUK_KEYCHAIN_PASSPHRASE"),
	}
}

func (o Options) fileDir() string {
	if o.FileDir != "" {
		return o.FileDir
	}
	if d := os.Getenv("SANDUK_FILE_DIR"); d != "" {
		return d
	}
	return defaultFileDir()
}

func (o Options) warn() io.Writer {
	if o.Warn != nil {
		return o.Warn
	}
	return os.Stderr
}

// Open opens the vault described by o.
func Open(o Options) (*Vault, error) {
	kr, err := openKeyring(o)
	if err != nil {
		return nil, err
	}
	return &Vault{kr: kr}, nil
}
