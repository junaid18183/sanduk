package cli

import (
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an entry from the vault",
		Long: "Remove an entry from the vault. A confirmation is required: with a terminal\n" +
			"it is asked interactively, and without one you must pass --yes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !yes {
				// Fail closed: without a terminal there is no way to confirm, so
				// require an explicit --yes rather than deleting silently in a
				// pipe or an agent session.
				if !vault.IsTerminal(os.Stdin) {
					return fmt.Errorf("refusing to remove %q without --yes (stdin is not a terminal)", name)
				}
				answer, err := promptLine(fmt.Sprintf("Remove %q? [y/N] ", name))
				if err != nil {
					return err
				}
				if answer != "y" && answer != "Y" && answer != "yes" {
					warnf("Aborted\n")
					return nil
				}
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			if err := v.Remove(name); err != nil {
				return err
			}

			// Best-effort metadata cleanup; ignore config errors here.
			_ = updateConfig(func(c *vault.Config) {
				delete(c.Secrets, name)
				delete(c.Groups, name)
			})

			warnf("Removed %q\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal non-interactively")
	cmd.ValidArgsFunction = completeEntryNames
	return cmd
}
