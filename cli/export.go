package cli

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExportCmd(a *app) *cobra.Command {
	var (
		format      string
		only        []string
		except      []string
		output      string
		force       bool
		materialize string
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
			"is given; it never appends. A symlink at that path is replaced by a\n" +
			"regular file rather than written through. Note that even\n" +
			"then an exported file is a plaintext copy of the secrets; prefer exec when the\n" +
			"values only need to reach one process.\n\n" +
			"A file secret has no value to print, only a path, so exporting one requires\n" +
			"--materialize DIR. Each is written there as a 0600 file and its variable is\n" +
			"set to that path. Those files are plaintext and are not cleaned up.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := a.resolveSelection(args, only, except)
			if err != nil {
				return err
			}
			env, err := a.exportEnv(sel, materialize)
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
					a.warnf("wrote %d variable(s) to %s\n", len(env), output)
				} else {
					a.warnf("wrote %d variable(s) to %s (mode 0600)\n", len(env), output)
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
	cmd.Flags().StringVar(&materialize, "materialize", "",
		"write file secrets as 0600 files in this directory and export their paths")
	cmd.Flags().BoolVar(&force, "force", false,
		"replace an existing --output file (a symlink is replaced, not written through)")
	cmd.ValidArgsFunction = a.completeSelector
	return cmd
}

// exportEnv flattens a selection for printing. A file secret has no printable
// value, so it is refused unless the caller named a directory to write it to.
func (a *app) exportEnv(sel selection, materialize string) (map[string]string, error) {
	if len(sel.files) == 0 {
		if materialize != "" {
			return nil, fmt.Errorf("--materialize applies to file secrets, and this selection has none")
		}
		return sel.env, nil
	}
	if materialize == "" {
		return nil, fmt.Errorf("the selection includes %d file secret(s) (%s), which have a path and no value; "+
			"pass --materialize DIR to write them out, or --except to drop them",
			len(sel.files), strings.Join(sortedPayloadKeys(sel.files), ", "))
	}
	paths, err := vault.MaterializeInto(materialize, sel.files)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(sel.env)+len(paths))
	for k, v := range sel.env {
		out[k] = v
	}
	for k, p := range paths {
		if _, clash := out[k]; clash {
			return nil, fmt.Errorf("%s is set by both a variable and a file secret", k)
		}
		out[k] = p
	}
	a.warnf("wrote %d file secret(s) to %s; they are plaintext and are not cleaned up\n",
		len(paths), materialize)
	return out, nil
}

// sortedPayloadKeys names a selection's file secrets, for messages.
func sortedPayloadKeys(files map[string]vault.FilePayload) []string {
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
