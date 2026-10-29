package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newRotateCmd(a *app) *cobra.Command {
	var (
		fromEnv string
		expires string
	)

	cmd := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Replace the value of an existing flat secret",
		Long: "Replace the value of an existing flat secret, keeping its name and\n" +
			"metadata intact. The value comes from a prompt, --from-env, or stdin.\n\n" +
			"--expires moves the rotation deadline at the same time, which is the\n" +
			"moment you actually know the new date:\n\n" +
			"  sanduk rotate stripe --expires 2027-04-01",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			v, err := a.openVault()
			if err != nil {
				return err
			}
			e, err := v.Get(name)
			if err != nil {
				return err
			}
			if e.Type != vault.TypeSecret {
				return fmt.Errorf("%q is a group; update its fields with \"sanduk group set -g %s KEY\"", name, name)
			}

			if expires != "" && expires != expiryNone {
				if _, err := vault.ParseExpiry(expires); err != nil {
					return err
				}
			}

			value, err := readSecret(fromEnv, "", true)
			if err != nil {
				return err
			}
			if err := v.AddSecret(name, value); err != nil {
				return err
			}

			if expires != "" {
				if err := a.setExpiry(name, expires); err != nil {
					return err
				}
			}

			a.warnf("Rotated secret %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the new value from this environment variable")
	cmd.Flags().StringVar(&expires, "expires", "", "move the rotation deadline to YYYY-MM-DD, or \"none\" to clear it")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}
