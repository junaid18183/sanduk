package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGroupLinkCmd(a *app) *cobra.Command {
	var group string

	cmd := &cobra.Command{
		Use:   "link [-g GROUP] [ENVVAR=]TARGET",
		Short: "Point a group field at a flat secret or another group's field",
		Long: "Link a group field to a value that already exists instead of storing a second\n" +
			"copy. Nothing is copied and no value is read into the group: the target is read\n" +
			"at resolve time, so rotating it changes what the group injects without any\n" +
			"re-sync step. The target stays the single source of truth.\n\n" +
			"TARGET is a flat secret, or a stored field of another group written\n" +
			"Group/FIELD. A bare flat secret name takes the variable it is already exposed\n" +
			"as (its --as mapping, or the upper-cased name). An ENVVAR= prefix sets the\n" +
			"variable explicitly, and is required for a Group/FIELD target, which has no\n" +
			"name to take it from:\n\n" +
			"  sanduk group link -g maktub elevenlabs\n" +
			"  sanduk group link -g maktub XI_KEY=elevenlabs\n" +
			"  sanduk group link -g maktub ELEVENLABS_API_KEY=ElevenLabs/elevenlabs_siddik\n\n" +
			"A field link may only point at a value the other group stores itself, never at\n" +
			"one of its links, so every link is one hop from its value and a cycle is\n" +
			"impossible. A variable is either stored in the group or linked, never both.\n" +
			"The group must already exist: create it with add-group.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			sel := parseSelector(args[0])
			envName := sel.env
			if envName == "" {
				if vault.IsFieldRef(sel.name) {
					return fmt.Errorf("%q names a group field; give the variable too, as ENVVAR=%s", sel.name, sel.name)
				}
				cfg, err := a.loadConfig()
				if err != nil {
					return err
				}
				envName = cfg.EnvNameForSecret(sel.name)
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := v.LinkGroupField(name, envName, sel.name); err != nil {
				return err
			}
			// Keep list accurate even if the metadata config was lost.
			if err := a.recordGroupMeta(name, ""); err != nil {
				return err
			}
			// Note the membership so a listing can keep the group's building
			// blocks out of the top level. It is a display hint: the link
			// itself lives in the entry, and resolution never reads this. A
			// field target is a group, which has no metadata record to hint
			// at, so it is skipped.
			if !vault.IsFieldRef(sel.name) {
				if err := a.updateConfig(func(c *vault.Config) { c.RecordLink(sel.name, name) }); err != nil {
					return err
				}
			}

			a.warnf("Linked %s in group %q -> %q\n", envName, name, sel.name)
			// A link adds a variable to the group like any other field.
			a.syncProjectOnly(name, []string{envName}, nil)
			return nil
		},
	}

	groupFlag(a, cmd, &group)
	return cmd
}

func newGroupUnlinkCmd(a *app) *cobra.Command {
	var group string

	cmd := &cobra.Command{
		Use:   "unlink [-g GROUP] ENVVAR [ENVVAR...]",
		Short: "Remove a link from a group, leaving its target untouched",
		Long: "Drop a linked variable from a group. Only the pointer is removed; the target\n" +
			"it addressed is not changed or deleted. Use \"sanduk group unset\" for a\n" +
			"variable whose value is stored in the group itself.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			// De-duplicate so `unlink A A` cannot fail on the second A, which
			// the first removal already deleted.
			keys := dedupeStrings(args)

			v, err := a.openVault()
			if err != nil {
				return err
			}
			// Read the targets before the links are gone, so the display
			// hints can be dropped with them.
			dropped := groupLinks(v, name)
			if err := v.UnlinkGroupField(name, keys); err != nil {
				return err
			}
			if err := a.updateConfig(func(c *vault.Config) {
				for _, k := range keys {
					if target := dropped[k]; target != "" {
						c.RemoveLink(target, name)
					}
				}
			}); err != nil {
				return err
			}

			a.warnf("Unlinked %d field(s) from group %q (the targets are untouched)\n",
				len(keys), name)
			a.syncProjectOnly(name, nil, keys)
			return nil
		},
	}
	groupFlag(a, cmd, &group)
	return cmd
}

// groupLinks returns a group's links, flat and field alike, or nil when the
// entry is missing or is not a group. It is used for display only.
func groupLinks(v *vault.Vault, name string) map[string]string {
	e, err := v.Get(name)
	if err != nil || e.Type != vault.TypeGroup {
		return nil
	}
	out := make(map[string]string, len(e.Links)+len(e.FieldLinks))
	for k, t := range e.Links {
		out[k] = t
	}
	for k, ref := range e.FieldLinks {
		out[k] = ref
	}
	return out
}

// linkRow renders the SOURCE column of "group show --long".
func linkRow(links map[string]string, key string) string {
	if target, ok := links[key]; ok {
		return "-> " + target
	}
	return "stored"
}
