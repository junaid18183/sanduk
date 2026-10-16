package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var (
		only   []string
		except []string
		list   bool
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
			"entry can be omitted altogether.\n\n" +
			"  sanduk exec warden -- pnpm dev\n" +
			"  sanduk exec BASE=base warden -- pnpm dev    # later selector wins\n" +
			"  sanduk exec warden --only DB_URL -- psql\n" +
			"  sanduk exec -- pnpm dev                     # entry from " + vault.ProjectFileName + "\n\n" +
			"--list prints the variable names that would be injected, never their values,\n" +
			"and runs nothing.\n\n" +
			"Without `--`, a single selector is accepted, then the command.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sels, command, err := splitExecArgs(args, cmd.ArgsLenAtDash())
			if err != nil {
				return err
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			// No selector means "the repository's default entry".
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

			if list {
				for _, k := range sortedMapKeys(env) {
					fmt.Fprintln(os.Stdout, k)
				}
				warnf("%d variable(s) would be injected; nothing was run\n", len(env))
				return nil
			}

			if err := vault.Exec(command[0], command[1:], env); err != nil {
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
	cmd.ValidArgsFunction = completeSelector
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
