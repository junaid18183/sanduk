package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newAddCmd(a *app) *cobra.Command {
	var (
		fromEnv     string
		fromFile    string
		envName     string
		description string
		replace     bool
	)

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add or update a flat secret",
		Long: "Add or update a flat secret. The value is read from an echo-disabled\n" +
			"prompt, an environment variable, a file, or stdin — never from argv.\n\n" +
			"An existing flat secret is replaced in place. Replacing a group with a flat\n" +
			"secret discards every field, so it requires --replace.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := vault.ValidateEntryName(name); err != nil {
				return err
			}
			// Check the name the secret will actually be exposed as: --as, an
			// existing mapping, or the upper-cased entry name. Without this,
			// `sanduk add ld_preload` would become LD_PRELOAD in every child.
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			exposed := envName
			if exposed == "" {
				exposed = cfg.EnvNameForSecret(name)
			}
			if err := vault.ValidateEnvName(exposed); err != nil {
				if envName == "" {
					return fmt.Errorf("%w; pass --as to choose a different variable name", err)
				}
				return err
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			// Updating a flat secret in place is what add is for; anything else
			// already stored under the name (a group, or an entry with no
			// metadata) would be destroyed, so that needs --replace.
			if _, isSecret := cfg.Secrets[name]; !isSecret {
				if err := a.refuseOverwrite(v, name, replace, ""); err != nil {
					return err
				}
			}

			value, err := readSecret(fromEnv, fromFile, true)
			if err != nil {
				return err
			}
			if err := v.AddSecret(name, value); err != nil {
				return err
			}

			if err := a.recordSecretMeta(name, envName, description); err != nil {
				return err
			}

			a.warnf("Added secret %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the value from this environment variable")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read the value from this file")
	cmd.Flags().StringVar(&envName, "as", "", "environment variable to expose the secret as")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry that is not a flat secret (e.g. a group)")
	cmd.MarkFlagsMutuallyExclusive("from-env", "from-file")
	return cmd
}
