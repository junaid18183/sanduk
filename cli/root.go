package cli

import (
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// cfgFile is bound to the persistent --config flag.
var cfgFile string

// backendFlag is bound to the persistent --backend flag.
var backendFlag string

// keychainFlag is bound to the persistent --keychain flag.
var keychainFlag string

// Execute builds and runs the root command.
func Execute() error {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "sanduk: "+err.Error())
		return err
	}
	return nil
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sanduk",
		Short: "sanduk is a vault for API keys and secrets in development",
		Long: "sanduk stores API keys and other secrets in your operating system's\n" +
			"secure keystore, then exposes them to the processes that need them — on\n" +
			"demand, without writing plaintext secrets to disk.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Resolve the backend settings from flag > env > config > default and
			// hand them to the vault package, which reads them from the
			// environment (see SANDUK_BACKEND / SANDUK_KEYCHAIN / SANDUK_SERVICE).
			s, err := resolveSettings()
			if err != nil {
				// A broken config must not brick every command, including
				// `sanduk config`, which is how you would inspect it. Remember
				// the failure and let each command report it.
				settingsErr = err
				resolved = nil
				return nil
			}
			resolved = s
			settingsErr = nil
			return exportSettings(s)
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default $XDG_CONFIG_HOME/sanduk/config.toml)")
	root.PersistentFlags().StringVar(&backendFlag, "backend", "",
		"keyring backend: keychain, keychain-biometric, secret-service, wincred, kwallet, pass, file, age")
	root.PersistentFlags().StringVar(&keychainFlag, "keychain", "",
		"macOS keychain to use (default \"login\"; a dedicated keychain is created on first write)")

	root.AddCommand(
		newAddCmd(),
		newAddGroupCmd(),
		newGroupCmd(),
		newGetCmd(),
		newRemoveCmd(),
		newRotateCmd(),
		newListCmd(),
		newExecCmd(),
		newExportCmd(),
		newImportCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)
	return root
}

// loadConfig loads the metadata config from the resolved location.
func loadConfig() (*vault.Config, string, error) {
	path := cfgFile
	if path == "" {
		path = vault.ConfigPath()
	}
	cfg, err := vault.LoadConfig(path)
	if err != nil {
		return nil, path, err
	}
	return cfg, path, nil
}

// updateConfig loads, mutates, and saves the metadata config.
func updateConfig(fn func(*vault.Config)) error {
	cfg, path, err := loadConfig()
	if err != nil {
		return err
	}
	fn(cfg)
	return vault.SaveConfig(path, cfg)
}
