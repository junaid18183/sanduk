package cli

import (
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "Print an entry's secrets as shell, dotenv, or JSON",
		Long: "Print an entry's secrets in a machine-readable form. Unlike get, export\n" +
			"is not gated on a terminal: it is the sanctioned interface for scripting\n" +
			"and .env files.\n\n" +
			"  CREDS_JSON=$(sanduk export aws-prod --format json)\n" +
			"  export AWS_ACCESS_KEY_ID=$(echo \"$CREDS_JSON\" | jq -r .AWS_ACCESS_KEY_ID)",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.New()
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			env, err := v.ResolveEnv(args[0], cfg)
			if err != nil {
				return err
			}
			return vault.FormatEnv(os.Stdout, env, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", vault.FormatShell,
		"output format: shell, dotenv or json")
	return cmd
}
