package cli

import (
	"fmt"
	"path/filepath"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newAllowCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allow [dir]",
		Short: "Approve a repository's .sanduk.toml for use by exec and export",
		Long: "Approve the nearest " + vault.ProjectFileName + " at or above dir (the current\n" +
			"directory by default). exec and export refuse to use a project file whose\n" +
			"entry has not been approved, because a repository can otherwise name any\n" +
			"vault entry and have its commands receive those secrets.\n\n" +
			"Approval records a hash of the file in trusted.toml, next to your config.\n" +
			"Editing the file revokes the approval, so re-run this after a legitimate change.\n\n" +
			"Every project file above this one applies here too, and each is approved\n" +
			"separately: this reports the ones that still need it, with the command for\n" +
			"each. Pass the directory to approve one without changing into it.",
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

			hash, err := vault.ApproveProject(a.trustPath(), proj)
			if err != nil {
				return err
			}

			// Show what is being approved, so the decision is made on the same
			// bytes whose hash is stored.
			a.warnf("Approved %s\n", path)
			a.warnf("entry %q (sha256 %s)\n", proj.Entry, hash[:12])
			a.warnf("Re-run after editing the file; edits revoke the approval.\n")

			// Approving a repository is also the moment its entry stops
			// being a loose entry and becomes that repository's, so listings
			// elsewhere can leave it out. Only a group gets a location: a
			// flat secret named by a project file is still usable anywhere.
			projDir := filepath.Dir(path)
			var recorded bool
			if err := a.updateConfig(func(c *vault.Config) {
				recorded = c.SetGroupLocation(proj.Entry, projDir)
			}); err != nil {
				return err
			}
			if recorded {
				a.warnf("Recorded %q as the group for %s\n", proj.Entry, projDir)
			}
			a.reportUnapprovedAbove(projDir)
			return nil
		},
	}
	return cmd
}

// reportUnapprovedAbove names the project files above dir that still need
// approving, one command per directory.
//
// Approval stays one deliberate decision per file — the hash is what makes
// editing a file revoke it, and approving several on one keystroke would
// hollow that out. What sanduk can do is stop the user discovering them one
// failed command at a time.
func (a *app) reportUnapprovedAbove(dir string) {
	chain, err := vault.LoadProjectChain(dir)
	if err != nil {
		return
	}
	for _, p := range chain {
		if p.Dir() == dir {
			continue
		}
		if ok, err := vault.IsProjectTrusted(a.trustPath(), p); err == nil && ok {
			continue
		}
		a.warnf("note: %s also applies here and is not approved; review it, then run %s\n",
			shortPath(p.Path()), allowHint(p.Dir()))
	}
}
