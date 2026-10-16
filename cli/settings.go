package cli

import (
	"os"
	"path/filepath"
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

// settings is the resolved configuration for one invocation.
type settings struct {
	ConfigPath string
	TrustPath  string
	FileDir    string

	Backend      string
	Keychain     string
	Service      string
	BackendFrom  settingSource
	KeychainFrom settingSource
	ServiceFrom  settingSource
}

// resolveSettings resolves the backend settings from flag > env > config >
// default. A config file that cannot be read is returned as the error, but the
// settings are still resolved from the flags and environment: a broken config
// must not make `--backend file` silently fall back to the platform keystore.
func (a *app) resolveSettings() (*settings, error) {
	path := a.configFlag
	if path == "" {
		path = defaultConfigPath()
	}
	cfg, cfgErr := vault.LoadConfig(path)
	if cfgErr != nil {
		cfg = &vault.Config{}
	}

	s := &settings{ConfigPath: path, TrustPath: trustPathFor(path), FileDir: defaultFileDir()}
	// An empty backend means "let the platform decide".
	s.Backend, s.BackendFrom = resolveSetting(a.backendFlag, "SANDUK_BACKEND", cfg.Backend, "")
	s.Keychain, s.KeychainFrom = resolveSetting(a.keychainFlag, "SANDUK_KEYCHAIN", cfg.Keychain, vault.DefaultKeychainName())
	s.Service, s.ServiceFrom = resolveSetting(a.serviceFlag, "SANDUK_SERVICE", cfg.Service, vault.DefaultService())

	// A keychain name is only meaningful on macOS. Warn rather than silently
	// ignore it, so a copied-over setting does not look effective.
	if runtime.GOOS != "darwin" && s.KeychainFrom != sourceDefault {
		a.warnf("sanduk: --keychain/SANDUK_KEYCHAIN is only used on macOS; ignoring %q\n", s.Keychain)
	}
	return s, cfgErr
}

// trustPathFor is the project trust store for a config path: SANDUK_TRUST, or
// trusted.toml next to the config, so it moves with --config.
func trustPathFor(configPath string) string {
	if p := os.Getenv("SANDUK_TRUST"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(configPath), vault.TrustFileName)
}

// defaultConfigPath is the metadata config location without --config:
// SANDUK_CONFIG, then $XDG_CONFIG_HOME/sanduk, then ~/.config/sanduk.
func defaultConfigPath() string {
	if p := os.Getenv("SANDUK_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(baseDir("XDG_CONFIG_HOME", ".config"), "sanduk", "config.toml")
}

// defaultFileDir is the age backend's directory: SANDUK_FILE_DIR, then
// $XDG_DATA_HOME/sanduk, then ~/.local/share/sanduk.
func defaultFileDir() string {
	if d := os.Getenv("SANDUK_FILE_DIR"); d != "" {
		return d
	}
	return filepath.Join(baseDir("XDG_DATA_HOME", filepath.Join(".local", "share")), "sanduk")
}

// baseDir returns $xdgVar, or homeRel under the home directory.
func baseDir(xdgVar, homeRel string) string {
	if x := os.Getenv(xdgVar); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return homeRel
	}
	return filepath.Join(home, homeRel)
}
