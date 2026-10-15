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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !yes && vault.IsTerminal(os.Stdin) {
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

	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}
