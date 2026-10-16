package vault

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/99designs/keyring"
)

const (
	defaultService = "sanduk"

	// defaultKeychainName is macOS's login keychain. 99designs/keyring turns it
	// into "login.keychain", which resolves through the keychain search list.
	defaultKeychainName = "login"
)

// Backends lists the accepted backend names, for flag help and errors.
var Backends = []string{"keychain", "keychain-biometric", "secret-service", "wincred", "kwallet", "pass", "file", "age"}

// DefaultService returns the keychain service name used when none is set.
func DefaultService() string { return defaultService }

// DefaultKeychainName returns the macOS keychain used when none is set.
func DefaultKeychainName() string { return defaultKeychainName }

// keychainTarget validates the macOS keychain name, defaulting to "login".
//
// Anything other than the default "login" makes keyring create and use a
// dedicated keychain (macOS only), which is unlocked as its own boundary. It is
// opt-in: the login keychain is already unlocked for a logged-in user, so it
// usually prompts less than a second keychain would.
func keychainTarget(name string) (string, error) {
	name = strings.TrimSpace(name)
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

// serviceName validates the keystore service name, defaulting to "sanduk". It
// also names the age file (<dir>/<service>.age), so like the keychain name it
// must be a plain name: a value such as "../../x" would put the vault anywhere.
func serviceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultService, nil
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("invalid service name %q: pass a name, not a path", name)
	}
	return name, nil
}

// openKeyring opens the backend o selects.
//
// When o.Backend is empty, exactly one native keystore is used — see
// defaultNativeBackend — and failing to open it is an error, never a fallback
// to another store. The library's own file backend is never selectable: its
// opener cannot fail, so it would shadow a missing keystore and then error on
// the first read. Only when the platform keystore is not compiled in at all
// does sanduk fall back to its own age-encrypted file, which is explicit about
// what it is and where it lives.
//
// 99designs/keyring's macOS Keychain backend is compiled only under
// `darwin && cgo`, so macOS builds must be made with CGO_ENABLED=1 to have an
// OS keystore at all.
func openKeyring(o Options) (keyring.Keyring, error) {
	service, err := serviceName(o.Service)
	if err != nil {
		return nil, err
	}

	keychain, err := keychainTarget(o.Keychain)
	if err != nil {
		return nil, err
	}

	warn := o.warn()
	agePath := func() (string, error) {
		if o.FileDir == "" {
			return "", errors.New("the age backend needs a file directory")
		}
		return filepath.Join(o.FileDir, service+".age"), nil
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
	if pw := o.KeychainPassphrase; pw != "" {
		cfg.KeychainPasswordFunc = func(string) (string, error) { return pw, nil }
	}

	backend := strings.TrimSpace(o.Backend)

	// The age and biometric backends are sanduk's own implementations; they are
	// not part of 99designs/keyring, so route them before delegating.
	switch backend {
	case "file", "age":
		path, err := agePath()
		if err != nil {
			return nil, err
		}
		return newAgeKeyring(path, o.passFunc(), warn), nil

	case "keychain-biometric":
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
			path, err := agePath()
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(warn,
				"sanduk: no OS keystore compiled in, using the encrypted file backend at %s\n", path)
			return newAgeKeyring(path, o.passFunc(), warn), nil
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
		return nil, fmt.Errorf("unknown backend %q (want %s)", backend, strings.Join(Backends, ", "))
	}

	cfg.AllowedBackends = allowed
	kr, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not open the %s keystore: %w", allowed[0], err)
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
