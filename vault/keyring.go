package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
	"golang.org/x/term"
)

const defaultService = "sanduk"

// OpenKeyring opens the configured keyring backend.
//
// The backend is selected with --backend / SANDUK_BACKEND (mirroring aws-vault).
// When unset, the platform default order from 99designs/keyring is used:
// macOS Keychain, Windows Credential Manager, Secret Service / KWallet, pass.
func OpenKeyring() (keyring.Keyring, error) {
	service := os.Getenv("SANDUK_SERVICE")
	if service == "" {
		service = defaultService
	}

	cfg := keyring.Config{
		ServiceName:             service,
		KeychainName:            "login",
		LibSecretCollectionName: "login",
		KWalletAppID:            service,
		KWalletFolder:           service,
		WinCredPrefix:           service,
		PassPrefix:              service,
	}

	backend := strings.TrimSpace(os.Getenv("SANDUK_BACKEND"))

	// The age backend is sanduk's own encrypted-file implementation; it is not
	// part of 99designs/keyring, so route it before delegating.
	switch backend {
	case "file", "age":
		dir := os.Getenv("SANDUK_FILE_DIR")
		if dir == "" {
			dir = defaultFileDir()
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		return newAgeKeyring(filepath.Join(dir, service+".age"), filePasswordFunc), nil

	case "keychain-biometric", "biometric":
		return newBiometricKeyring(service)
	}

	var allowed []keyring.BackendType
	switch backend {
	case "":
		// Use the platform default.
	case "keychain":
		allowed = []keyring.BackendType{keyring.KeychainBackend}
	case "secret-service":
		allowed = []keyring.BackendType{keyring.SecretServiceBackend}
	case "wincred":
		allowed = []keyring.BackendType{keyring.WinCredBackend}
	case "kwallet":
		allowed = []keyring.BackendType{keyring.KWalletBackend}
	case "pass":
		allowed = []keyring.BackendType{keyring.PassBackend}
	default:
		return nil, fmt.Errorf("unknown backend %q (want keychain, keychain-biometric, secret-service, wincred, kwallet, pass, file or age)", backend)
	}

	if allowed != nil {
		cfg.AllowedBackends = allowed
	}
	return keyring.Open(cfg)
}

func defaultFileDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "sanduk")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".sanduk", "data")
	}
	return filepath.Join(home, ".local", "share", "sanduk")
}

// filePasswordFunc supplies the passphrase for the encrypted file backend. It
// reads SANDUK_FILE_PASSPHRASE when set (useful for scripts and tests) and
// otherwise prompts on the terminal with echo disabled.
func filePasswordFunc(prompt string) (string, error) {
	if p := os.Getenv("SANDUK_FILE_PASSPHRASE"); p != "" {
		return p, nil
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
