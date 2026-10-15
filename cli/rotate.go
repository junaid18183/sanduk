package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newRotateCmd() *cobra.Command {
	var (
		fromEnv  string
		fromFile string
	)

	cmd := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Replace the value of an existing flat secret",
		Long: "Replace the value of an existing flat secret, keeping its name and\n" +
			"metadata intact.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			v, err := vault.New()
			if err != nil {
				return err
			}
			e, err := v.Get(name)
			if err != nil {
				return err
			}
			if e.Type != vault.TypeSecret {
				return fmt.Errorf("%q is a group; update it with add-group", name)
			}

			value, err := readSecret(fromEnv, fromFile, true)
			if err != nil {
				return err
			}
			if err := v.AddSecret(name, value); err != nil {
				return err
			}

			warnf("Rotated secret %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the new value from this environment variable")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read the new value from this file")
	return cmd
}
