package cli

import (
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		fromEnv     string
		fromFile    string
		envName     string
		description string
	)

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add or update a flat secret",
		Long: "Add or update a flat secret. The value is read from an echo-disabled\n" +
			"prompt, an environment variable, a file, or stdin — never from argv.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			value, err := readSecret(fromEnv, fromFile, true)
			if err != nil {
				return err
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			if err := v.AddSecret(name, value); err != nil {
				return err
			}

			if err := recordSecretMeta(name, envName, description); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Added secret %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the value from this environment variable")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read the value from this file")
	cmd.Flags().StringVar(&envName, "env", "", "environment variable to expose the secret as")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	return cmd
}
