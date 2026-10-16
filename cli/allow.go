package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newAllowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allow [dir]",
		Short: "Approve a repository's .sanduk.toml for use by exec and export",
		Long: "Approve the nearest " + vault.ProjectFileName + " at or above dir (the current\n" +
			"directory by default). exec and export refuse to use a project file whose\n" +
			"entry has not been approved, because a repository can otherwise name any\n" +
			"vault entry and have its commands receive those secrets.\n\n" +
			"Approval records a hash of the file in trusted.toml, next to your config.\n" +
			"Editing the file revokes the approval, so re-run this after a legitimate change.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}

			path, proj, err := vault.LoadProject(dir)
			if err != nil {
				return err
			}
			if proj == nil {
				return fmt.Errorf("no %s found in %s or its parents", vault.ProjectFileName, dir)
			}

			hash, err := vault.ApproveProject(trustPath(), proj)
			if err != nil {
				return err
			}

			// Show what is being approved, so the decision is made on the same
			// bytes whose hash is stored.
			warnf("Approved %s\n", path)
			warnf("entry %q (sha256 %s)\n", proj.Entry, hash[:12])
			warnf("Re-run after editing the file; edits revoke the approval.\n")
			return nil
		},
	}
	return cmd
}
