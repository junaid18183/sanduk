package cli

import (
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		format string
		only   []string
	)

	cmd := &cobra.Command{
		Use:   "export [selector...]",
		Short: "Print an entry's secrets as shell, dotenv, or JSON",
		Long: "Print an entry's secrets in a machine-readable form. Unlike get, export\n" +
			"is not gated on a terminal: it is the sanctioned interface for scripting\n" +
			"and .env files.\n\n" +
			"Selectors are the same as for exec, and several may be merged. With a\n" +
			vault.ProjectFileName + " in the repository the entry can be omitted.\n\n" +
			"  CREDS_JSON=$(sanduk export aws-prod --format json)\n" +
			"  docker compose --env-file <(sanduk export warden --format dotenv) up\n" +
			"  eval \"$(sanduk export --format shell)\"   # entry from " + vault.ProjectFileName + "\n\n" +
			"Note that exporting into a shell puts the secrets in every child process\n" +
			"of that shell. Prefer exec when an agent or editor shares the terminal.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.New()
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			sels := args
			if len(sels) == 0 {
				entry, projOnly, err := defaultEntry()
				if err != nil {
					return err
				}
				sels = []string{entry}
				if len(only) == 0 {
					only = projOnly
				}
			}

			env, err := resolveSelectors(v, cfg, parseSelectors(sels))
			if err != nil {
				return err
			}
			env, err = filterOnly(env, only)
			if err != nil {
				return err
			}
			return vault.FormatEnv(os.Stdout, env, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", vault.FormatShell,
		"output format: shell, dotenv or json")
	cmd.Flags().StringArrayVar(&only, "only", nil,
		"export only these keys (comma-separated, repeatable)")
	return cmd
}
