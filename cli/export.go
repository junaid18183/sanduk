package cli

import (
	"bytes"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		format string
		only   []string
		except []string
		output string
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
			"Prefer --output over a shell redirect: it writes the file with mode 0600,\n" +
			"whereas `export ... >> .env` inherits your umask (often 0644). Note that even\n" +
			"then an exported file is a plaintext copy of the secrets; prefer exec when the\n" +
			"values only need to reach one process.",
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
			env = filterExcept(env, except)

			if output != "" {
				var buf bytes.Buffer
				if err := vault.FormatEnv(&buf, env, format); err != nil {
					return err
				}
				if err := vault.WriteFileAtomic(output, buf.Bytes(), 0o600); err != nil {
					return err
				}
				warnf("wrote %d variable(s) to %s (mode 0600)\n", len(env), output)
				return nil
			}
			return vault.FormatEnv(os.Stdout, env, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", vault.FormatShell,
		"output format: shell, dotenv or json")
	cmd.Flags().StringArrayVar(&only, "only", nil,
		"export only these keys (comma-separated, repeatable)")
	cmd.Flags().StringArrayVar(&except, "except", nil,
		"exclude these keys (comma-separated, repeatable)")
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"write to this file with mode 0600 instead of stdout")
	cmd.ValidArgsFunction = completeSelector
	return cmd
}
