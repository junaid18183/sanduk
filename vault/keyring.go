package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// DefaultService returns the keychain service name used when none is set.
func DefaultService() string { return defaultService }

// DefaultKeychainName returns the macOS keychain used when none is set.
func DefaultKeychainName() string { return defaultKeychainName }

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

// serviceName resolves the keystore service name from SANDUK_SERVICE. It also
// names the age file (<dir>/<service>.age), so like the keychain name it must be
// a plain name: a value such as "../../x" would put the vault anywhere.
func serviceName() (string, error) {
	name := strings.TrimSpace(os.Getenv("SANDUK_SERVICE"))
	if name == "" {
		return defaultService, nil
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("invalid service name %q: pass a name, not a path", name)
	}
	return name, nil
}

// OpenKeyring opens the configured keyring backend.
//
// The backend is selected with --backend / SANDUK_BACKEND (or the config file).
// When unset, exactly one native keystore is used — see defaultNativeBackend —
// and failing to open it is an error, never a fallback to another store. The
// library's own file backend is never selectable: its opener cannot fail, so it
// would shadow a missing keystore and then error on the first read. Only when
// the platform keystore is not compiled in at all does sanduk fall back to its
// own age-encrypted file, which is explicit about what it is and where it lives.
//
// Note that 99designs/keyring's macOS Keychain backend is compiled only under
// `darwin && cgo`, so macOS builds must be made with CGO_ENABLED=1 to have an
// OS keystore at all.
func OpenKeyring() (keyring.Keyring, error) {
	service, err := serviceName()
	if err != nil {
		return nil, err
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
		native, ok := defaultNativeBackend(runtime.GOOS, nativeBackends())
		if !ok {
			// The platform's keystore is not compiled in — for example a macOS
			// build made with CGO_ENABLED=0, which cannot include the Keychain
			// backend. Only this case falls back to the encrypted file.
			path := agePath(service)
			fmt.Fprintf(os.Stderr,
				"sanduk: no OS keystore compiled in, using the encrypted file backend at %s\n", path)
			return newAgeKeyring(path, filePasswordFunc), nil
		}
		// Exactly one backend: keyring.Open silently moves on to the next
		// allowed backend when one fails to open, so offering several would let
		// a transient D-Bus error put new secrets in kwallet or pass instead,
		// splitting the vault across two stores. A failure is an error.
		allowed = []keyring.BackendType{native}
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
	if err != nil {
		return nil, err
	}
	return kr, nil
}

// defaultNativeBackend picks the one keystore used when no backend is set: the
// Keychain on macOS, the Credential Manager on Windows and the Secret Service
// elsewhere. kwallet and pass are never chosen implicitly; select them with
// --backend. It reports false when that keystore is not compiled in.
func defaultNativeBackend(goos string, available []keyring.BackendType) (keyring.BackendType, bool) {
	want := keyring.SecretServiceBackend
	switch goos {
	case "darwin":
		want = keyring.KeychainBackend
	case "windows":
		want = keyring.WinCredBackend
	}
	for _, b := range available {
		if b == want {
			return want, true
		}
	}
	return "", false
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
