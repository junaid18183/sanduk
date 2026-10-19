package cli

import (
	"fmt"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExecCmd(a *app) *cobra.Command {
	var (
		only        []string
		except      []string
		list        bool
		materialize string
	)

	cmd := &cobra.Command{
		Use:   "exec [selector...] [--] <command> [args...]",
		Short: "Run a command with an entry's secrets injected as environment variables",
		Long: "Run a command with secrets injected into its environment. The secrets exist\n" +
			"only inside the child process, never in the calling shell.\n\n" +
			"Selectors say what to inject, and are merged left to right:\n\n" +
			"  warden                  every field of the group (or a flat secret's variable)\n" +
			"  OPENAI_API_KEY=openai   one flat secret, under the given variable name\n\n" +
			"Use `--` to separate selectors from the command; that is what makes several\n" +
			"selectors unambiguous. With a " + vault.ProjectFileName + " in the repository the\n" +
			"entry can be omitted altogether, once the file has been approved with\n" +
			"\"sanduk allow\".\n\n" +
			"  sanduk exec warden -- pnpm dev\n" +
			"  sanduk exec BASE=base warden -- pnpm dev    # later selector wins\n" +
			"  sanduk exec warden --only DB_URL -- psql\n" +
			"  sanduk exec -- pnpm dev                     # entry from " + vault.ProjectFileName + "\n\n" +
			"--list prints the variable names that would be injected, never their values,\n" +
			"and runs nothing.\n\n" +
			"A file secret is injected as a path. By default its bytes go to a temporary\n" +
			"file that is unlinked at once and handed to the child as /dev/fd/N, so\n" +
			"nothing readable is left on disk and the data is freed when the child exits.\n" +
			"Use --materialize DIR for a tool that insists on a real path; those files\n" +
			"persist, because exec replaces this process and leaves nobody to delete them.\n\n" +
			"Without `--`, a single selector is accepted, then the command.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sels, command, err := splitExecArgs(args, cmd.ArgsLenAtDash())
			if err != nil {
				return err
			}

			// No selector means "the repository's default entry".
			sel, err := a.resolveSelection(sels, only, except)
			if err != nil {
				return err
			}

			if list {
				for _, k := range sel.names() {
					fmt.Fprintln(cmd.OutOrStdout(), k)
				}
				a.warnf("%d variable(s) would be injected; nothing was run\n", len(sel.names()))
				return nil
			}

			if materialize != "" && len(sel.files) == 0 {
				return fmt.Errorf("--materialize applies to file secrets, and this selection has none")
			}
			if len(sel.files) > 0 && materialize != "" {
				a.warnf("writing %d file secret(s) to %s; they outlive the command, so remove them when done\n",
					len(sel.files), materialize)
			}

			opts := vault.ExecOptions{Env: sel.env, Files: sel.files, MaterializeDir: materialize}
			if err := vault.Exec(command[0], command[1:], opts); err != nil {
				if strings.Contains(command[0], "=") {
					return fmt.Errorf("%w (hint: selectors go before `--`; %q looks like an ENVVAR=entry binding)",
						err, command[0])
				}
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&only, "only", nil,
		"inject only these keys (comma-separated, repeatable)")
	cmd.Flags().StringArrayVar(&except, "except", nil,
		"exclude these keys (comma-separated, repeatable)")
	cmd.Flags().BoolVar(&list, "list", false,
		"print the variable names that would be injected, without running anything")
	cmd.Flags().StringVar(&materialize, "materialize", "",
		"write file secrets as real 0600 files in this directory instead of /dev/fd")
	cmd.ValidArgsFunction = a.completeSelector
	return cmd
}

// splitExecArgs separates the selectors from the command.
//
// With `--`, everything before it is a selector and everything after it is the
// command — that is what makes multiple selectors unambiguous. Without `--`,
// only the first argument is a selector, preserving the original form:
//
//	sanduk exec warden -- pnpm dev
//	sanduk exec warden pnpm dev
func splitExecArgs(args []string, dash int) (sels []string, command []string, err error) {
	const usage = "usage: sanduk exec [selector...] -- <command> [args...]"

	if dash >= 0 {
		if dash == len(args) {
			return nil, nil, fmt.Errorf("%s", usage)
		}
		return args[:dash], args[dash:], nil
	}
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("%s", usage)
	}
	return args[:1], args[1:], nil
}
