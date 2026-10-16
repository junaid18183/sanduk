package cli

import (
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
	Backend      string
	Keychain     string
	Service      string
	BackendFrom  settingSource
	KeychainFrom settingSource
	ServiceFrom  settingSource
}

// resolveSettings loads the config and resolves the backend settings from it.
func resolveSettings() (*settings, error) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, err
	}

	s := &settings{}
	// An empty backend means "let the platform decide", which the vault package
	// also expresses as an empty SANDUK_BACKEND.
	s.Backend, s.BackendFrom = resolveSetting(backendFlag, "SANDUK_BACKEND", cfg.Backend, "")
	s.Keychain, s.KeychainFrom = resolveSetting(keychainFlag, "SANDUK_KEYCHAIN", cfg.Keychain, vault.DefaultKeychainName())
	s.Service, s.ServiceFrom = resolveSetting(serviceFlag, "SANDUK_SERVICE", cfg.Service, vault.DefaultService())

	// A keychain name is only meaningful on macOS. Warn rather than silently
	// ignore it, so a copied-over setting does not look effective.
	if runtime.GOOS != "darwin" && s.KeychainFrom != sourceDefault {
		warnf("sanduk: --keychain/SANDUK_KEYCHAIN is only used on macOS; ignoring %q\n", s.Keychain)
	}
	return s, nil
}

// exportSettings makes the resolved settings visible to the vault package,
// which reads them from the environment.
func exportSettings(s *settings) error {
	values := []struct {
		name  string
		value string
	}{
		{"SANDUK_BACKEND", s.Backend},
		{"SANDUK_KEYCHAIN", s.Keychain},
		{"SANDUK_SERVICE", s.Service},
	}
	for _, v := range values {
		if v.value == "" {
			continue
		}
		if err := os.Setenv(v.name, v.value); err != nil {
			return err
		}
	}
	return nil
}

// resolved and settingsErr hold the outcome of PersistentPreRunE so commands can
// report it without resolving a second time. Resolving again would be wrong:
// exportSettings has already written the values into the environment, so the
// original source would be reported as "env" for everything.
var (
	resolved    *settings
	settingsErr error
)
