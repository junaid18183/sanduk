package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
	"golang.org/x/term"
)

const (
	defaultService = "sanduk"

	// defaultKeychainName is macOS's login keychain. 99designs/keyring turns it
	// into "login.keychain", which resolves through the keychain search list.
	defaultKeychainName = "login"
)

// keychainTarget resolves the keychain to use from SANDUK_KEYCHAIN.
//
// Anything other than the default "login" makes keyring create and use a
// dedicated keychain (macOS only), which is unlocked as its own boundary. It is
// opt-in: the login keychain is already unlocked for a logged-in user, so it
// usually prompts less than a second keychain would.
func keychainTarget() (string, error) {
	name := strings.TrimSpace(os.Getenv("SANDUK_KEYCHAIN"))
	if name == "" {
		return defaultKeychainName, nil
	}
	// keyring appends ".keychain" to this and hands it to the Security
	// framework, so accept a name and reject anything path-shaped.
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid keychain name %q: pass a name, not a path", name)
	}
	return name, nil
}

// OpenKeyring opens the configured keyring backend.
//
// The backend is selected with --backend / SANDUK_BACKEND (mirroring aws-vault).
// When unset, only the platform's *native* keystores are considered. The
// library's own file backend is deliberately excluded: its opener never fails,
// so it would silently shadow a missing native keystore and then error on the
// first read. If no native keystore is available we fall back to sanduk's own
// age-encrypted file, which is explicit about what it is and where it lives.
//
// Note that 99designs/keyring's macOS Keychain backend is compiled only under
// `darwin && cgo`, so macOS builds must be made with CGO_ENABLED=1 to have an
// OS keystore at all.
func OpenKeyring() (keyring.Keyring, error) {
	service := os.Getenv("SANDUK_SERVICE")
	if service == "" {
		service = defaultService
	}

	keychain, err := keychainTarget()
	if err != nil {
		return nil, err
	}

	cfg := keyring.Config{
		ServiceName:             service,
		KeychainName:            keychain,
		LibSecretCollectionName: "login",
		KWalletAppID:            service,
		KWalletFolder:           service,
		WinCredPrefix:           service,
		PassPrefix:              service,
	}

	// Only consulted when keyring has to create a dedicated keychain; when it
	// is unset the user is prompted for a new keychain password.
	if pw := os.Getenv("SANDUK_KEYCHAIN_PASSPHRASE"); pw != "" {
		cfg.KeychainPasswordFunc = func(string) (string, error) { return pw, nil }
	}

	backend := strings.TrimSpace(os.Getenv("SANDUK_BACKEND"))

	// The age and biometric backends are sanduk's own implementations; they are
	// not part of 99designs/keyring, so route them before delegating.
	switch backend {
	case "file", "age":
		return newAgeKeyring(agePath(service), filePasswordFunc), nil

	case "keychain-biometric", "biometric":
		return newBiometricKeyring(service)
	}

	var allowed []keyring.BackendType
	switch backend {
	case "":
		allowed = nativeBackends()
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

	cfg.AllowedBackends = allowed
	kr, err := keyring.Open(cfg)
	if err == nil {
		return kr, nil
	}
	if backend != "" {
		return nil, err
	}

	// No native keystore is available — for example a macOS build made with
	// CGO_ENABLED=0, which cannot include the Keychain backend.
	path := agePath(service)
	fmt.Fprintf(os.Stderr,
		"sanduk: no OS keystore available, using the encrypted file backend at %s\n", path)
	return newAgeKeyring(path, filePasswordFunc), nil
}

// nativeBackends returns the compiled-in native keystores, excluding the
// library's file backend. The result is never nil, so keyring.Open cannot fall
// back to its own defaults.
func nativeBackends() []keyring.BackendType {
	out := []keyring.BackendType{}
	for _, b := range keyring.AvailableBackends() {
		if b == keyring.FileBackend {
			continue
		}
		out = append(out, b)
	}
	return out
}

// agePath is the location of the age-encrypted vault file.
func agePath(service string) string {
	dir := os.Getenv("SANDUK_FILE_DIR")
	if dir == "" {
		dir = defaultFileDir()
	}
	return filepath.Join(dir, service+".age")
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
