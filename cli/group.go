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
		Short: "Inspect and edit the fields of a grouped entry",
		Long: "A group stores several named fields as one vault entry, where each field name\n" +
			"is an environment variable name. Use add-group to create one, and these\n" +
			"subcommands to edit it a field at a time.",
	}
	cmd.AddCommand(newGroupSetCmd(a), newGroupUnsetCmd(a), newGroupShowCmd(a), newGroupGetCmd(a),
		newGroupLinkCmd(a), newGroupUnlinkCmd(a))
	return cmd
}

func newGroupSetCmd(a *app) *cobra.Command {
	var (
		fromEnv    bool
		fromDotenv string
		descFlag   string
	)

	cmd := &cobra.Command{
		Use:   "set <group> [KEY...]",
		Short: "Add or replace individual fields of a group",
		Long: "Merge fields into a group. Only the named fields change; every other\n" +
			"field keeps its value, and a group that does not exist yet is created with\n" +
			"these fields.\n\n" +
			"Values are never taken from the command line, where they would be visible in\n" +
			"process listings. They come from an echo-disabled prompt, from stdin when a\n" +
			"single KEY is named and stdin is a pipe, from environment variables, or from\n" +
			"a .env file:\n\n" +
			"  sanduk group set warden DB_PASSWORD\n" +
			"  echo \"$new\" | sanduk group set warden DB_PASSWORD\n" +
			"  sanduk group set warden --from-env DB_PASSWORD STRIPE_KEY\n" +
			"  sanduk group set warden --from-dotenv patch.env\n\n" +
			"When a field already exists its current value is shown redacted, and the new\n" +
			"value is asked for twice, so a typo cannot be stored.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			keys := args[1:]

			for _, k := range keys {
				if err := vault.ValidateEnvName(k); err != nil {
					return err
				}
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}

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
				m = parsed

			case fromEnv:
				if len(keys) == 0 {
					return fmt.Errorf("--from-env needs at least one KEY")
				}
				for _, k := range keys {
					val, ok := os.LookupEnv(k)
					if !ok || val == "" {
						return fmt.Errorf("environment variable %s is not set", k)
					}
					m[k] = val
				}

			default:
				if len(keys) == 0 {
					return fmt.Errorf("no fields given; name at least one KEY, or use --from-dotenv")
				}
				// Show the redacted current value only to someone at the
				// prompt; a piped run would put it in a log.
				var current map[string]string
				if isTerminal(os.Stdin) {
					current = currentGroupFields(v, name)
				}
				for _, k := range keys {
					if cur, ok := current[k]; ok {
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

			a.warnf("Updated %d field(s) in group %q\n", len(m), name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&fromEnv, "from-env", false,
		"read each value from the environment variable of the same name")
	cmd.Flags().StringVar(&fromDotenv, "from-dotenv", "", "merge the fields of a .env file")
	cmd.Flags().StringVar(&descFlag, "description", "", "set the group's description")
	cmd.MarkFlagsMutuallyExclusive("from-env", "from-dotenv")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

// currentGroupFields returns the existing fields of a group, or nil when the
// entry does not exist or is not a group. It is used only to show a redacted
// "current" value before an overwrite.
func currentGroupFields(v *vault.Vault, name string) map[string]string {
	e, err := v.Get(name)
	if err != nil || e.Type != vault.TypeGroup {
		return nil
	}
	return e.Fields
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

func newGroupUnsetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <group> KEY [KEY...]",
		Short: "Remove fields from a group",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			// De-duplicate so `unset g A A` cannot fail on the second A, which
			// the first removal already deleted.
			keys := dedupeStrings(args[1:])

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := v.UnsetGroupFields(name, keys); err != nil {
				return err
			}

			a.warnf("Removed %d field(s) from group %q\n", len(keys), name)
			return nil
		},
	}
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

func newGroupShowCmd(a *app) *cobra.Command {
	var long bool

	cmd := &cobra.Command{
		Use:   "show <group>",
		Short: "List a group's field names (never its values)",
		Long: "Print one field name per line, so the output composes with grep, xargs and\n" +
			"--only. Linked fields are listed like any other: a consumer of the list\n" +
			"cares which variables the group contributes, not where each value is kept.\n" +
			"Values are never printed; use \"sanduk group get\" for one value.\n\n" +
			"--long adds a table showing which fields are links and what they point at.\n" +
			"It is a separate flag so the default output stays safe to pipe.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := a.openVault()
			if err != nil {
				return err
			}
			names, err := v.GroupFieldNames(args[0])
			if err != nil {
				return err
			}
			links := groupLinks(v, args[0])

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

			a.warnf("%d field(s) in group %q", len(names), args[0])
			if len(links) > 0 {
				a.warnf(", %d linked", len(links))
			}
			a.warnf("\n")
			return nil
		},
	}
	cmd.Flags().BoolVar(&long, "long", false, "show which fields are links and their targets")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

func newGroupGetCmd(a *app) *cobra.Command {
	var (
		reveal bool
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "get <group> KEY",
		Short: "Print one field of a group (redacted unless --reveal)",
		Long: "Print a single field's value, so reviewing one secret does not dump the whole\n" +
			"group. Like get, it refuses to print to a non-terminal unless --force is\n" +
			"given, and it prints a redacted value unless --reveal is passed.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := requireTerminal(out, force); err != nil {
				return err
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			val, linkedTo, err := v.GroupFieldValue(args[0], args[1])
			if err != nil {
				return err
			}
			if linkedTo != "" {
				a.warnf("%s is linked to secret %q\n", args[1], linkedTo)
			}
			_, err = fmt.Fprintln(out, revealOrRedact(val, reveal))
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "allow printing to a non-terminal")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "print the full value instead of a redacted one")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}
