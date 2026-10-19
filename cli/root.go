package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// Execute builds and runs the root command.
func Execute() error {
	if err := newRootCmd(newApp()).Execute(); err != nil {
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

func newRootCmd(a *app) *cobra.Command {
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
			a.stderr = cmd.ErrOrStderr()
			// Resolve flag > env > config > default now. A broken config is
			// remembered rather than fatal: `sanduk config` is how you would
			// inspect it, and the commands that need the config report it.
			a.resolved()
			return nil
		},
	}

	root.PersistentFlags().StringVar(&a.configFlag, "config", "",
		"config file (default $XDG_CONFIG_HOME/sanduk/config.toml)")
	root.PersistentFlags().StringVar(&a.backendFlag, "backend", "",
		"keyring backend: "+strings.Join(vault.Backends, ", "))
	root.PersistentFlags().StringVar(&a.keychainFlag, "keychain", "",
		"macOS keychain to use (default \"login\"; a dedicated keychain is created on first write)")
	root.PersistentFlags().StringVar(&a.serviceFlag, "service", "",
		"keystore service name, also the age file name (default \"sanduk\")")

	root.AddCommand(
		newAddCmd(a),
		newAddGroupCmd(a),
		newAddFileCmd(a),
		newGroupCmd(a),
		newGetCmd(a),
		newRemoveCmd(a),
		newRotateCmd(a),
		newListCmd(a),
		newExecCmd(a),
		newExportCmd(a),
		newImportCmd(a),
		newAllowCmd(a),
		newDoctorCmd(a),
		newScanCmd(a),
		newMCPCmd(a),
		newConfigCmd(a),
		newVersionCmd(a),
	)
	return root
}
