package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/junaid18183/sanduk/vault"
)

// app is one invocation of sanduk: the global flags, the settings resolved from
// them, and where status messages go. Every command closes over it, so there is
// no package-level state and each test runs against a fresh app.
type app struct {
	// Global flags.
	configFlag   string
	backendFlag  string
	keychainFlag string
	serviceFlag  string

	// stderr receives status messages, keeping stdout machine-readable.
	stderr io.Writer

	// settings is resolved once, on first use (shell completion does not run
	// PersistentPreRunE, so this cannot rely on it). settingsErr records a
	// config file that could not be read; the settings are still resolved from
	// flags and environment.
	settings    *settings
	settingsErr error
}

func newApp() *app { return &app{stderr: os.Stderr} }

// warnf writes a status message to stderr.
func (a *app) warnf(format string, args ...any) {
	fmt.Fprintf(a.stderr, format, args...)
}

// resolved returns the settings for this invocation, resolving them on first
// use. The result is never nil.
func (a *app) resolved() *settings {
	if a.settings == nil {
		a.settings, a.settingsErr = a.resolveSettings()
	}
	return a.settings
}

// configPath is the metadata config in use.
func (a *app) configPath() string { return a.resolved().ConfigPath }

// trustPath is the project trust store in use (see trustPathFor).
func (a *app) trustPath() string { return a.resolved().TrustPath }

// openVault opens the vault with the resolved settings. It refuses when the
// config is unreadable: every command that opens the vault also reads or writes
// its metadata, and failing after a backend write would leave the two out of
// step. `sanduk config` does not open the vault, so it still works.
func (a *app) openVault() (*vault.Vault, error) {
	s := a.resolved()
	if a.settingsErr != nil {
		return nil, fmt.Errorf("config is unreadable (inspect it with \"sanduk config\"): %w", a.settingsErr)
	}
	return vault.Open(a.options(s))
}

// options turns the resolved settings into what the vault package needs. The
// passphrases are secrets, so they only ever come from the environment or a
// prompt, never from config or flags.
func (a *app) options(s *settings) vault.Options {
	return vault.Options{
		Backend:            s.Backend,
		Keychain:           s.Keychain,
		Service:            s.Service,
		FileDir:            s.FileDir,
		PassFunc:           agePassphrase,
		KeychainPassphrase: os.Getenv("SANDUK_KEYCHAIN_PASSPHRASE"),
		Warn:               a.stderr,
	}
}

// agePassphrase supplies the age backend's passphrase: SANDUK_FILE_PASSPHRASE
// when set (scripts, tests), otherwise an echo-off prompt.
func agePassphrase(prompt string) (string, error) {
	if p := os.Getenv("SANDUK_FILE_PASSPHRASE"); p != "" {
		return p, nil
	}
	return promptSecret(prompt)
}

// loadConfig loads the metadata config from the resolved location.
func (a *app) loadConfig() (*vault.Config, error) {
	return vault.LoadConfig(a.configPath())
}

// updateConfig loads, mutates and saves the metadata config under a lock.
func (a *app) updateConfig(fn func(*vault.Config)) error {
	return vault.UpdateConfig(a.configPath(), func(c *vault.Config) error {
		fn(c)
		return nil
	})
}

// recordSecretMeta marks name as a flat secret in the metadata config, so list
// stays accurate without reading values from the keystore.
func (a *app) recordSecretMeta(name, env, desc string) error {
	return a.updateConfig(func(c *vault.Config) { c.RecordSecret(name, env, desc) })
}

// recordGroupMeta marks name as a group in the metadata config.
func (a *app) recordGroupMeta(name, desc string) error {
	return a.updateConfig(func(c *vault.Config) { c.RecordGroup(name, desc) })
}

// expiryNone is the --expires value that clears a recorded deadline.
const expiryNone = "none"

// setExpiry records an entry's rotation deadline, or clears it for "none". The
// metadata record must already exist: an expiry describes an entry sanduk
// knows about, and creating a record here would have to guess its type.
func (a *app) setExpiry(name, date string) error {
	if date == expiryNone {
		date = ""
	} else if _, err := vault.ParseExpiry(date); err != nil {
		return err
	}
	var recorded bool
	if err := a.updateConfig(func(c *vault.Config) { recorded = c.SetExpiry(name, date) }); err != nil {
		return err
	}
	if !recorded {
		return fmt.Errorf("%q has no metadata record to attach an expiry to "+
			"(fix: sanduk doctor --metadata --fix)", name)
	}
	return nil
}

// refuseOverwrite fails when an entry called name already exists and replace
// is not set: destructive writes are never the default. Existence is checked by
// listing names, so no value is decrypted and no Touch ID prompt fires; the
// entry's type for the message comes from the metadata config. hint, when
// non-empty, is offered before "pass --replace".
func (a *app) refuseOverwrite(v *vault.Vault, name string, replace bool, hint string) error {
	if replace {
		return nil
	}
	exists, err := v.Exists(name)
	if err != nil || !exists {
		return err
	}
	return fmt.Errorf("%q already exists%s; %spass --replace to overwrite it", name, a.describeKind(name), hint)
}

// describeKind names an entry's type from the metadata config, for messages.
func (a *app) describeKind(name string) string {
	cfg, err := a.loadConfig()
	if err != nil {
		return ""
	}
	if _, ok := cfg.Groups[name]; ok {
		return " as a group"
	}
	if _, ok := cfg.Secrets[name]; ok {
		return " as a flat secret"
	}
	return ""
}
