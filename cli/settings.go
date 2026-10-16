package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/junaid18183/sanduk/vault"
)

// settingSource records where a resolved setting came from, so that
// `sanduk config` can explain the effective configuration instead of leaving
// the user to work out which of flag/env/config/default won.
type settingSource string

const (
	sourceFlag    settingSource = "flag"
	sourceEnv     settingSource = "env"
	sourceConfig  settingSource = "config"
	sourceDefault settingSource = "default"
)

// resolveSetting applies the precedence flag > environment > config > default.
func resolveSetting(flagVal, envName, cfgVal, def string) (string, settingSource) {
	switch {
	case flagVal != "":
		return flagVal, sourceFlag
	case os.Getenv(envName) != "":
		return os.Getenv(envName), sourceEnv
	case cfgVal != "":
		return cfgVal, sourceConfig
	default:
		return def, sourceDefault
	}
}

// settings is the resolved backend configuration.
type settings struct {
	ConfigPath   string
	Backend      string
	Keychain     string
	Service      string
	BackendFrom  settingSource
	KeychainFrom settingSource
	ServiceFrom  settingSource
}

// resolveSettings resolves the backend settings from flag > env > config >
// default. A config file that cannot be read is reported as the error, but the
// settings are still resolved from the flags and environment: a broken config
// must not make `--backend file` silently fall back to the platform keystore.
func resolveSettings() (*settings, error) {
	path := cfgFile
	if path == "" {
		path = vault.ConfigPath()
	}
	cfg, cfgErr := vault.LoadConfig(path)
	if cfgErr != nil {
		cfg = &vault.Config{}
	}

	s := &settings{ConfigPath: path}
	// An empty backend means "let the platform decide".
	s.Backend, s.BackendFrom = resolveSetting(backendFlag, "SANDUK_BACKEND", cfg.Backend, "")
	s.Keychain, s.KeychainFrom = resolveSetting(keychainFlag, "SANDUK_KEYCHAIN", cfg.Keychain, vault.DefaultKeychainName())
	s.Service, s.ServiceFrom = resolveSetting(serviceFlag, "SANDUK_SERVICE", cfg.Service, vault.DefaultService())

	// A keychain name is only meaningful on macOS. Warn rather than silently
	// ignore it, so a copied-over setting does not look effective.
	if runtime.GOOS != "darwin" && s.KeychainFrom != sourceDefault {
		warnf("sanduk: --keychain/SANDUK_KEYCHAIN is only used on macOS; ignoring %q\n", s.Keychain)
	}
	return s, cfgErr
}

// options turns the resolved settings into what the vault package needs. The
// age passphrase and the keychain-creation passphrase are secrets, so they only
// ever come from the environment (or a prompt), never from config or flags.
func (s *settings) options() vault.Options {
	o := vault.OptionsFromEnv()
	o.Backend = s.Backend
	o.Keychain = s.Keychain
	o.Service = s.Service
	return o
}

// resolved and settingsErr hold the settings for the running command. They are
// resolved once, by PersistentPreRunE or on first use (shell completion does not
// run PersistentPreRunE), and reset by newRootCmd.
var (
	resolved    *settings
	settingsErr error
)

// currentSettings returns the resolved settings, resolving them on first use.
// The result is never nil; settingsErr says whether the config was readable.
func currentSettings() *settings {
	if resolved == nil {
		resolved, settingsErr = resolveSettings()
	}
	return resolved
}

// configPath is the metadata config in use (--config, SANDUK_CONFIG, default).
func configPath() string { return currentSettings().ConfigPath }

// trustPath is the project trust store for the config in use.
func trustPath() string { return vault.TrustPathFor(configPath()) }

// openVault opens the vault with the resolved settings. It refuses when the
// config is unreadable: every command that opens the vault also reads or writes
// its metadata, and failing after a backend write would leave the two out of
// step. `sanduk config` does not open the vault, so it still works.
func openVault() (*vault.Vault, error) {
	s := currentSettings()
	if settingsErr != nil {
		return nil, fmt.Errorf("config is unreadable (inspect it with \"sanduk config\"): %w", settingsErr)
	}
	return vault.Open(s.options())
}
