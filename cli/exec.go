package cli

import (
	"fmt"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <name> [--] <command> [args...]",
		Short: "Run a command with an entry's secrets injected as environment variables",
		Long: "Run a command with secrets from a vault entry injected into its\n" +
			"environment. The secrets exist only inside the child process, never in\n" +
			"the calling shell.\n\n" +
			"Bind a single secret to a specific variable with an ad-hoc mapping:\n\n" +
			"  sanduk exec OPENAI_API_KEY=openai -- aider",
		Args:               cobra.MinimumNArgs(2),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			selector, command, err := splitExecArgs(args, cmd.ArgsLenAtDash())
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

			env, err := resolveSelector(v, cfg, selector)
			if err != nil {
				return err
			}

			return vault.Exec(command[0], command[1:], env)
		},
	}
	return cmd
}

// splitExecArgs separates the selector from the command, tolerating an optional
// `--` separator.
func splitExecArgs(args []string, dash int) (selector string, command []string, err error) {
	if dash >= 0 {
		if dash != 1 {
			return "", nil, fmt.Errorf("usage: sanduk exec <name> -- <command> [args...]")
		}
		return args[0], args[dash:], nil
	}
	return args[0], args[1:], nil
}

// resolveSelector resolves either a vault entry name or an ad-hoc
// ENVVAR=entryname mapping into environment variables.
func resolveSelector(v *vault.Vault, cfg *vault.Config, selector string) (map[string]string, error) {
	if envName, name, ok := strings.Cut(selector, "="); ok && envName != "" {
		e, err := v.Get(name)
		if err != nil {
			return nil, err
		}
		if e.Type != vault.TypeSecret {
			return nil, fmt.Errorf("%q is a group; ad-hoc binding requires a flat secret", name)
		}
		return map[string]string{envName: e.Value}, nil
	}
	return v.ResolveEnv(selector, cfg)
}
