package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newRemoveCmd(a *app) *cobra.Command {
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
				if !isTerminal(os.Stdin) {
					return fmt.Errorf("refusing to remove %q without --yes (stdin is not a terminal)", name)
				}
				answer, err := promptLine(fmt.Sprintf("Remove %q? [y/N] ", name))
				if err != nil {
					return err
				}
				if answer != "y" && answer != "Y" && answer != "yes" {
					a.warnf("Aborted\n")
					return nil
				}
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			removeErr := v.Remove(name)
			if removeErr != nil && !errors.Is(removeErr, vault.ErrNotFound) {
				return removeErr
			}

			// Clean up the metadata even when the backend entry is already gone,
			// so an orphaned record does not linger (and resurface as a stale env
			// mapping for a later secret of the same name).
			hadMeta := false
			if err := a.updateConfig(func(c *vault.Config) {
				_, s := c.Secrets[name]
				_, g := c.Groups[name]
				hadMeta = s || g
				c.Forget(name)
			}); err != nil {
				a.warnf("warning: removed %q but could not update the config: %v\n", name, err)
			}

			if removeErr != nil {
				if !hadMeta {
					return removeErr
				}
				a.warnf("%q was not in the vault; removed its leftover metadata\n", name)
				return nil
			}
			a.warnf("Removed %q\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal non-interactively")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}
