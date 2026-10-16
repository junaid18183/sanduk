package cli

import (
	"errors"
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
			exposed := envName
			if exposed == "" {
				cfg, _, err := loadConfig()
				if err != nil {
					return err
				}
				exposed = cfg.EnvNameForSecret(name)
			}
			if err := vault.ValidateEnvName(exposed); err != nil {
				if envName == "" {
					return fmt.Errorf("%w; pass --as to choose a different variable name", err)
				}
				return err
			}

			v, err := openVault()
			if err != nil {
				return err
			}
			if existing, gerr := v.Get(name); gerr == nil {
				if existing.Type != vault.TypeSecret && !replace {
					return fmt.Errorf("%q is a %s; pass --replace to overwrite it with a flat secret",
						name, existing.Type)
				}
			} else if !errors.Is(gerr, vault.ErrNotFound) {
				return gerr
			}

			value, err := readSecret(fromEnv, fromFile, true)
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
	cmd.Flags().StringVar(&envName, "as", "", "environment variable to expose the secret as")
	cmd.Flags().StringVar(&envName, "env", "", "deprecated: use --as")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an entry of a different type (e.g. a group)")
	cmd.MarkFlagsMutuallyExclusive("from-env", "from-file")
	_ = cmd.Flags().MarkDeprecated("env", "use --as")
	return cmd
}
