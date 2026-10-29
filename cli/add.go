package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newAddCmd(a *app) *cobra.Command {
	var (
		fromEnv     string
		envName     string
		description string
		expires     string
		replace     bool
	)

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new flat secret",
		Long: "Add a flat secret. The value is read from an echo-disabled prompt, an\n" +
			"environment variable, or stdin, never from argv. To store a file's contents\n" +
			"as a value, pipe it in: sanduk add NAME < key.txt. Credential files are\n" +
			"stored with add-file instead.\n\n" +
			"An existing entry is never overwritten silently. To change a flat secret's\n" +
			"value use rotate; --replace overwrites any entry, including a group, and\n" +
			"discards what it held.",
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
			// Overwriting an entry destroys it, so that needs --replace. The
			// type is read from the backend, not the metadata, which can
			// drift: this costs one read of an existing entry (a Touch ID
			// prompt on keychain-biometric), but the check fails closed.
			if !replace {
				if err := refuseExisting(v, name); err != nil {
					return err
				}
			}

			// Validate the date before prompting: a typo should not cost the
			// user an echo-off paste.
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

			if err := a.recordSecretMeta(name, envName, description); err != nil {
				return err
			}
			if expires != "" {
				if err := a.setExpiry(name, expires); err != nil {
					return err
				}
			}

			a.warnf("Added secret %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the value from this environment variable")
	cmd.Flags().StringVar(&envName, "as", "", "environment variable to expose the secret as")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&expires, "expires", "", "rotation deadline as YYYY-MM-DD, or \"none\" to clear it")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry of any type")
	return cmd
}

// refuseExisting fails when name already exists in the vault, whatever its
// type. A missing entry is fine: add creates it.
func refuseExisting(v *vault.Vault, name string) error {
	exists, err := v.Exists(name)
	if err != nil || !exists {
		return err
	}
	e, err := v.Get(name)
	if err != nil {
		return err
	}
	if e.Type == vault.TypeSecret {
		return fmt.Errorf("%q already exists; use \"sanduk rotate %s\" to change its value, or --replace to overwrite it", name, name)
	}
	return fmt.Errorf("%q already exists as a %s; pass --replace to overwrite it", name, e.Type)
}
