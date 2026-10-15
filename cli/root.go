package cli

import (
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// cfgFile is bound to the persistent --config flag.
var cfgFile string

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
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default $XDG_CONFIG_HOME/sanduk/config.toml)")

	root.AddCommand(
		newAddCmd(),
		newAddGroupCmd(),
		newGetCmd(),
		newRemoveCmd(),
		newRotateCmd(),
		newListCmd(),
		newExecCmd(),
		newExportCmd(),
		newImportCmd(),
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
