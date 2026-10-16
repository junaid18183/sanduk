package cli

import (
	"errors"
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

// serviceFlag is bound to the persistent --service flag.
var serviceFlag string

// Execute builds and runs the root command.
func Execute() error {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		// A bare exit status (the child of exec on Windows) has already
		// reported itself; anything else is ours to print.
		var exit *vault.ExitError
		if !errors.As(err, &exit) || exit.Err != nil {
			fmt.Fprintln(os.Stderr, "sanduk: "+err.Error())
		}
		return err
	}
	return nil
}

func newRootCmd() *cobra.Command {
	resolved, settingsErr = nil, nil
	root := &cobra.Command{
		Use:   "sanduk",
		Short: "sanduk is a vault for API keys and secrets in development",
		Long: "sanduk stores API keys and other secrets in your operating system's\n" +
			"secure keystore, then exposes them to the processes that need them — on\n" +
			"demand, without writing plaintext secrets to disk.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Resolve flag > env > config > default once. A broken config is
			// remembered rather than fatal: `sanduk config` is how you would
			// inspect it, and the commands that need the config report it.
			resolved, settingsErr = resolveSettings()
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default $XDG_CONFIG_HOME/sanduk/config.toml)")
	root.PersistentFlags().StringVar(&backendFlag, "backend", "",
		"keyring backend: keychain, keychain-biometric, secret-service, wincred, kwallet, pass, file, age")
	root.PersistentFlags().StringVar(&keychainFlag, "keychain", "",
		"macOS keychain to use (default \"login\"; a dedicated keychain is created on first write)")
	root.PersistentFlags().StringVar(&serviceFlag, "service", "",
		"keystore service name, also the age file name (default \"sanduk\")")

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
		newAllowCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)
	return root
}

// loadConfig loads the metadata config from the resolved location.
func loadConfig() (*vault.Config, string, error) {
	path := configPath()
	cfg, err := vault.LoadConfig(path)
	if err != nil {
		return nil, path, err
	}
	return cfg, path, nil
}

// updateConfig loads, mutates and saves the metadata config under a lock.
func updateConfig(fn func(*vault.Config)) error {
	return vault.UpdateConfig(configPath(), func(c *vault.Config) error {
		fn(c)
		return nil
	})
}
