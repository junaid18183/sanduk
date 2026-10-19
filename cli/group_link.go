package cli

import (
	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGroupLinkCmd(a *app) *cobra.Command {
	var descFlag string

	cmd := &cobra.Command{
		Use:   "link <group> [ENVVAR=]<secret>",
		Short: "Point a group field at an existing flat secret",
		Long: "Link a group field to a flat secret instead of storing a second copy of the\n" +
			"value. Nothing is copied and no value is read into the group: the secret is\n" +
			"read at resolve time, so rotating it changes what the group injects without\n" +
			"any re-sync step. The secret stays the single source of truth.\n\n" +
			"With a bare secret name the variable is the one that secret is already\n" +
			"exposed as (its --as mapping, or the upper-cased name), which is the same\n" +
			"name \"sanduk export <secret>\" prints. An ENVVAR= prefix binds it under a\n" +
			"different name instead:\n\n" +
			"  sanduk group link maktub elevenlabs\n" +
			"  sanduk group link maktub XI_KEY=elevenlabs\n\n" +
			"A link may only point at a flat secret, which is what makes a cycle\n" +
			"impossible. A variable is either stored in the group or linked, never both.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			group := args[0]
			sel := parseSelector(args[1])

			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			envName := sel.env
			if envName == "" {
				envName = cfg.EnvNameForSecret(sel.name)
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := v.LinkGroupField(group, envName, sel.name); err != nil {
				return err
			}
			// Keep list accurate even if the metadata config was lost, and
			// record a group created by its first link.
			if err := a.recordGroupMeta(group, descFlag); err != nil {
				return err
			}

			a.warnf("Linked %s in group %q -> secret %q\n", envName, group, sel.name)
			return nil
		},
	}

	cmd.Flags().StringVar(&descFlag, "description", "", "set the group's description")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

func newGroupUnlinkCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlink <group> KEY [KEY...]",
		Short: "Remove a link from a group, leaving the secret itself untouched",
		Long: "Drop a linked variable from a group. Only the pointer is removed; the flat\n" +
			"secret it addressed is not changed or deleted. Use \"sanduk group unset\" for\n" +
			"a variable whose value is stored in the group itself.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			group := args[0]
			// De-duplicate so `unlink g A A` cannot fail on the second A, which
			// the first removal already deleted.
			keys := dedupeStrings(args[1:])

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := v.UnlinkGroupField(group, keys); err != nil {
				return err
			}

			a.warnf("Unlinked %d field(s) from group %q (the linked secrets are untouched)\n",
				len(keys), group)
			return nil
		},
	}
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

// groupLinks returns a group's links, or nil when the entry is missing or is
// not a group. It is used for display only.
func groupLinks(v *vault.Vault, name string) map[string]string {
	e, err := v.Get(name)
	if err != nil || e.Type != vault.TypeGroup {
		return nil
	}
	return e.Links
}

// linkRow renders the SOURCE column of "group show --long".
func linkRow(links map[string]string, key string) string {
	if target, ok := links[key]; ok {
		return "-> " + target
	}
	return "stored"
}
