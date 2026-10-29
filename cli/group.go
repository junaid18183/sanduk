package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGroupCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Edit the fields of a group",
		Long: "A group stores several named fields as one vault entry, where each field name\n" +
			"is an environment variable name. Create one with add-group, then edit it a\n" +
			"field at a time with these subcommands.\n\n" +
			"Each takes -g GROUP. Without it, the group is the entry of the approved\n" +
			".sanduk.toml in this directory, the same default exec and export use.",
	}
	cmd.AddCommand(newGroupListCmd(a), newGroupAddCmd(a), newGroupRemoveCmd(a), newGroupRotateCmd(a), newGroupShowCmd(a),
		newGroupLinkCmd(a), newGroupUnlinkCmd(a))
	return cmd
}

// groupFlag adds -g/--group to a group subcommand. The group is a flag rather
// than a positional because the field names these commands also take would
// make a single argument ambiguous: "get KEY" or "get GROUP"?
func groupFlag(a *app, cmd *cobra.Command, group *string) {
	cmd.Flags().StringVarP(group, "group", "g", "",
		"group to edit (default: the entry of the approved .sanduk.toml here)")
	_ = cmd.RegisterFlagCompletionFunc("group", func(c *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return a.completeEntryNames(c, nil, toComplete)
	})
}

// resolveGroup returns the group a command edits: the -g flag if given, else the
// entry of the nearest .sanduk.toml above this directory. That file must be
// approved, the same as for exec and export, because it decides what is edited.
func (a *app) resolveGroup(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	chain, err := vault.LoadProjectChain(".")
	if err != nil {
		return "", err
	}
	if len(chain) == 0 {
		return "", fmt.Errorf("no group given: pass -g GROUP, or run \"sanduk init\" in a repository")
	}
	nearest := chain[len(chain)-1]
	if err := a.requireTrustedProject(nearest); err != nil {
		return "", err
	}
	return nearest.Entry, nil
}

// requireGroup fails unless name is an existing group. Only add-group creates a
// group, so a mistyped -g cannot quietly start a new one.
func requireGroup(v *vault.Vault, name string) error {
	exists, err := v.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no group %q; create it with \"sanduk add-group %s\"", name, name)
	}
	return nil
}

func newGroupAddCmd(a *app) *cobra.Command {
	var (
		group      string
		fromEnv    string
		fromDotenv string
		descFlag   string
		expires    string
		replace    bool
	)

	cmd := &cobra.Command{
		Use:   "add [-g GROUP] [KEY...]",
		Short: "Add fields to an existing group",
		Long: "Add fields to a group. Only the named fields change; every other field keeps\n" +
			"its value. A field that already exists is refused unless --replace is given,\n" +
			"the same rule add applies to an existing entry. The group must already exist:\n" +
			"create it with add-group.\n\n" +
			"Values are never taken from the command line, where they would be visible in\n" +
			"process listings. They come from an echo-disabled prompt, from stdin when a\n" +
			"single KEY is named and stdin is a pipe, from one environment variable, or\n" +
			"from a .env file:\n\n" +
			"  sanduk group add -g warden DB_PASSWORD\n" +
			"  echo \"$new\" | sanduk group add -g warden DB_PASSWORD\n" +
			"  sanduk group add -g warden DB_PASSWORD --from-env NEW_DB_PASSWORD\n" +
			"  sanduk group add -g warden --from-dotenv patch.env\n" +
			"  sanduk group add -g warden DB_PASSWORD --replace\n\n" +
			"With --replace, a field that already exists shows its current value redacted,\n" +
			"and the new value is asked for twice, so a typo cannot be stored.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			keys := args
			for _, k := range keys {
				if err := vault.ValidateEnvName(k); err != nil {
					return err
				}
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := requireGroup(v, name); err != nil {
				return err
			}
			current := currentGroupFields(v, name)

			m := map[string]string{}
			switch {
			case fromDotenv != "":
				if len(keys) > 0 {
					return fmt.Errorf("--from-dotenv takes no field names")
				}
				parsed, err := readDotenvFile(fromDotenv)
				if err != nil {
					return err
				}
				if err := refuseExistingFields(name, current, sortedMapKeys(parsed), replace); err != nil {
					return err
				}
				m = parsed

			case fromEnv != "":
				if len(keys) != 1 {
					return fmt.Errorf("--from-env fills exactly one KEY; name it, and nothing else")
				}
				val, ok := os.LookupEnv(fromEnv)
				if !ok || val == "" {
					return fmt.Errorf("environment variable %s is not set or empty", fromEnv)
				}
				if err := refuseExistingFields(name, current, keys, replace); err != nil {
					return err
				}
				m[keys[0]] = val

			default:
				if len(keys) == 0 {
					return fmt.Errorf("no fields given; name at least one KEY, or use --from-dotenv")
				}
				// Refuse before prompting, so nothing is typed for a field
				// that would then be turned away.
				if err := refuseExistingFields(name, current, keys, replace); err != nil {
					return err
				}
				// Show the redacted current value only to someone at the
				// prompt; a piped run would put it in a log.
				showCurrent := isTerminal(os.Stdin)
				for _, k := range keys {
					if cur, ok := current[k]; ok && showCurrent {
						a.warnf("%s: %s (current)\n", k, redact(cur))
					}
					val, err := readGroupFieldValue(k, len(keys) == 1)
					if err != nil {
						return err
					}
					m[k] = val
				}
			}

			if err := v.MergeGroup(name, m); err != nil {
				return err
			}
			// Keep list accurate even if the metadata config was lost.
			if err := a.recordGroupMeta(name, descFlag); err != nil {
				return err
			}
			if expires != "" {
				if err := a.setExpiry(name, expires); err != nil {
					return err
				}
			}

			a.warnf("Added %d field(s) to group %q\n", len(m), name)
			// A project file that lists the keys it expects should not need
			// hand-editing every time the group grows one.
			a.syncProjectOnly(name, sortedMapKeys(m), nil)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "",
		"read the value of the one KEY from this environment variable")
	cmd.Flags().StringVar(&fromDotenv, "from-dotenv", "", "add the fields of a .env file")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite fields that already exist")
	cmd.Flags().StringVar(&descFlag, "description", "", "set the group's description")
	cmd.Flags().StringVar(&expires, "expires", "", "rotation deadline as YYYY-MM-DD, or \"none\" to clear it")
	cmd.MarkFlagsMutuallyExclusive("from-env", "from-dotenv")
	groupFlag(a, cmd, &group)
	return cmd
}

// currentGroupFields returns the existing fields of a group, or nil when the
// entry does not exist or is not a group. A nil result refuses nothing: the
// merge that follows reads the entry again and reports any real error.
func currentGroupFields(v *vault.Vault, name string) map[string]string {
	e, err := v.Get(name)
	if err != nil || e.Type != vault.TypeGroup {
		return nil
	}
	return e.Fields
}

// refuseExistingFields fails when one of keys is already a field of the group
// and replace is not set. Overwriting a stored value destroys it, so that needs
// the flag, the same rule add applies to an existing entry.
func refuseExistingFields(group string, current map[string]string, keys []string, replace bool) error {
	if replace {
		return nil
	}
	var taken []string
	for _, k := range keys {
		if _, ok := current[k]; ok {
			taken = append(taken, k)
		}
	}
	if len(taken) > 0 {
		return fmt.Errorf("group %q already has %s; pass --replace to overwrite them",
			group, strings.Join(taken, ", "))
	}
	return nil
}

// readGroupFieldValue reads one field's new value. With a single key named and a
// piped stdin it reads the value from stdin, matching `add`; otherwise it
// prompts with echo disabled and asks for confirmation, so a typo cannot be
// stored.
func readGroupFieldValue(key string, allowStdin bool) (string, error) {
	if allowStdin && !isTerminal(os.Stdin) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		val := strings.TrimRight(string(b), "\r\n")
		if val == "" {
			return "", fmt.Errorf("no value provided on stdin for %s", key)
		}
		return val, nil
	}

	val, err := promptSecret(key + ": ")
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", fmt.Errorf("no value provided for %s", key)
	}
	again, err := promptSecret(key + " (confirm): ")
	if err != nil {
		return "", err
	}
	if again != val {
		return "", fmt.Errorf("values do not match for %s", key)
	}
	return val, nil
}

func newGroupRemoveCmd(a *app) *cobra.Command {
	var group string

	cmd := &cobra.Command{
		Use:   "remove [-g GROUP] KEY [KEY...]",
		Short: "Remove fields from a group",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			// De-duplicate so `unset A A` cannot fail on the second A, which
			// the first removal already deleted.
			keys := dedupeStrings(args)

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := v.UnsetGroupFields(name, keys); err != nil {
				return err
			}

			a.warnf("Removed %d field(s) from group %q\n", len(keys), name)
			a.syncProjectOnly(name, nil, keys)
			return nil
		},
	}
	groupFlag(a, cmd, &group)
	return cmd
}

func newGroupRotateCmd(a *app) *cobra.Command {
	var (
		group   string
		fromEnv string
	)

	cmd := &cobra.Command{
		Use:   "rotate [-g GROUP] KEY",
		Short: "Replace the value of one existing field of a group",
		Long: "Replace the stored value of a field that already exists, keeping its name\n" +
			"and the other fields intact. The value comes from a prompt, --from-env, or\n" +
			"stdin, the same as group add. A field the group does not store is refused:\n" +
			"use group add for a new one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			key := args[0]
			if err := vault.ValidateEnvName(key); err != nil {
				return err
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			// Check before prompting, so a value is not typed for a field that
			// would then be refused. RotateGroupField checks again on write.
			if err := requireGroup(v, name); err != nil {
				return err
			}
			if _, ok := currentGroupFields(v, name)[key]; !ok {
				return fmt.Errorf("group %q has no stored field %q; add it with \"sanduk group add -g %s %s\"", name, key, name, key)
			}
			value, err := readSecret(fromEnv, "", true)
			if err != nil {
				return err
			}
			if err := v.RotateGroupField(name, key, value); err != nil {
				return err
			}

			a.warnf("Rotated %s in group %q\n", key, name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromEnv, "from-env", "", "read the new value from this environment variable")
	groupFlag(a, cmd, &group)
	return cmd
}

func newGroupShowCmd(a *app) *cobra.Command {
	var (
		group string
		long  bool
	)

	cmd := &cobra.Command{
		Use:   "show [-g GROUP]",
		Short: "List a group's field names (never its values)",
		Long: "Print one field name per line, so the output composes with grep, xargs and\n" +
			"--only. Linked fields are listed like any other: a consumer of the list\n" +
			"cares which variables the group contributes, not where each value is kept.\n" +
			"Values are never printed; use \"sanduk get -g GROUP --field KEY\" for one.\n\n" +
			"--long adds a table showing which fields are links and what they point at.\n" +
			"It is a separate flag so the default output stays safe to pipe.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.resolveGroup(group)
			if err != nil {
				return err
			}
			v, err := a.openVault()
			if err != nil {
				return err
			}
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			// One read, not one per thing we want from it. The entry carries
			// both its field names and its links, and on a backend that
			// prompts per read — a dedicated keychain, or keychain-biometric
			// — a second read is a second prompt for the same entry.
			vars, err := v.Variables(name, cfg)
			if err != nil {
				return err
			}
			if vars.Type != vault.TypeGroup {
				return fmt.Errorf("%q is a %s, not a group", name, vars.Type)
			}
			names, links := vars.Names, vars.Links

			if long {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(tw, "KEY\tSOURCE")
				for _, n := range names {
					fmt.Fprintf(tw, "%s\t%s\n", n, linkRow(links, n))
				}
				if err := tw.Flush(); err != nil {
					return err
				}
			} else {
				for _, n := range names {
					fmt.Fprintln(cmd.OutOrStdout(), n)
				}
			}

			a.warnf("%d field(s) in group %q", len(names), name)
			if len(links) > 0 {
				a.warnf(", %d linked", len(links))
			}
			a.warnf("\n")
			return nil
		},
	}
	cmd.Flags().BoolVar(&long, "long", false, "show which fields are links and their targets")
	groupFlag(a, cmd, &group)
	return cmd
}
