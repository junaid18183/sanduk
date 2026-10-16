package cli

import (
	"bytes"
	"fmt"
	"os"
	"runtime"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		format string
		only   []string
		except []string
		output string
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "export [selector...]",
		Short: "Print an entry's secrets as shell, dotenv, or JSON",
		Long: "Print an entry's secrets in a machine-readable form. Unlike get, export\n" +
			"is not gated on a terminal: it is the sanctioned interface for scripting\n" +
			"and .env files.\n\n" +
			"Selectors are the same as for exec, and several may be merged. With a\n" +
			vault.ProjectFileName + " in the repository the entry can be omitted, once the\n" +
			"file has been approved with \"sanduk allow\".\n\n" +
			"  CREDS_JSON=$(sanduk export aws-prod --format json)\n" +
			"  docker compose --env-file <(sanduk export warden --format dotenv) up\n" +
			"  eval \"$(sanduk export --format shell)\"   # entry from " + vault.ProjectFileName + "\n\n" +
			"Prefer --output over a shell redirect: it writes the file with mode 0600\n" +
			"(on Unix), whereas `export ... > .env` inherits your umask (often 0644).\n" +
			"--output replaces the whole file and refuses an existing one unless --force\n" +
			"is given; it never appends. Note that even\n" +
			"then an exported file is a plaintext copy of the secrets; prefer exec when the\n" +
			"values only need to reach one process.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveSelection(args, only, except)
			if err != nil {
				return err
			}

			if output != "" {
				if _, err := os.Lstat(output); err == nil && !force {
					return fmt.Errorf("%s already exists; pass --force to replace it", output)
				} else if err != nil && !os.IsNotExist(err) {
					return err
				}
				var buf bytes.Buffer
				if err := vault.FormatEnv(&buf, env, format); err != nil {
					return err
				}
				if err := vault.WriteFileAtomic(output, buf.Bytes(), 0o600); err != nil {
					return err
				}
				if runtime.GOOS == "windows" {
					warnf("wrote %d variable(s) to %s\n", len(env), output)
				} else {
					warnf("wrote %d variable(s) to %s (mode 0600)\n", len(env), output)
				}
				return nil
			}
			return vault.FormatEnv(cmd.OutOrStdout(), env, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", vault.FormatShell,
		"output format: shell, dotenv or json")
	cmd.Flags().StringArrayVar(&only, "only", nil,
		"export only these keys (comma-separated, repeatable)")
	cmd.Flags().StringArrayVar(&except, "except", nil,
		"exclude these keys (comma-separated, repeatable)")
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"write to this file (mode 0600) instead of stdout")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing --output file")
	cmd.ValidArgsFunction = completeSelector
	return cmd
}
