package cli

import (
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Inspect and edit the fields of a grouped entry",
		Long: "A group stores several named fields as one vault entry, where each field name\n" +
			"is an environment variable name. Use add-group to create one, and these\n" +
			"subcommands to edit it a field at a time.",
	}
	cmd.AddCommand(newGroupSetCmd(), newGroupUnsetCmd(), newGroupShowCmd())
	return cmd
}

func newGroupSetCmd() *cobra.Command {
	var (
		fromEnv  bool
		fromFile string
	)

	cmd := &cobra.Command{
		Use:   "set <group> [KEY...]",
		Short: "Add or replace individual fields of a group",
		Long: "Merge fields into a group. Only the named fields change; every other\n" +
			"field keeps its value, and a group that does not exist yet is created with\n" +
			"these fields.\n\n" +
			"Values are never taken from the command line, where they would be visible in\n" +
			"process listings. They come from an echo-disabled prompt, from environment\n" +
			"variables, or from a file:\n\n" +
			"  sanduk group set warden DB_PASSWORD\n" +
			"  sanduk group set warden --from-env DB_PASSWORD STRIPE_KEY\n" +
			"  sanduk group set warden --from-file patch.env",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			keys := args[1:]

			m := map[string]string{}
			switch {
			case fromFile != "":
				if len(keys) > 0 {
					return fmt.Errorf("--from-file takes no field names")
				}
				parsed, err := readDotenvFile(fromFile)
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
					return fmt.Errorf("no fields given; name at least one KEY, or use --from-file")
				}
				for _, k := range keys {
					val, err := promptSecret(k + ": ")
					if err != nil {
						return err
					}
					if val == "" {
						return fmt.Errorf("no value provided for %s", k)
					}
					m[k] = val
				}
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			if err := v.MergeGroup(name, m); err != nil {
				return err
			}
			// Keep list accurate even if the metadata config was lost.
			if err := recordGroupMeta(name, ""); err != nil {
				return err
			}

			warnf("Updated %d field(s) in group %q\n", len(m), name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&fromEnv, "from-env", false,
		"read each value from the environment variable of the same name")
	cmd.Flags().StringVar(&fromFile, "from-file", "",
		"merge the fields of a .env file")
	return cmd
}

func newGroupUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <group> KEY [KEY...]",
		Short: "Remove fields from a group",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			keys := args[1:]

			v, err := vault.New()
			if err != nil {
				return err
			}
			if err := v.UnsetGroupFields(name, keys); err != nil {
				return err
			}

			warnf("Removed %d field(s) from group %q\n", len(keys), name)
			return nil
		},
	}
}

func newGroupShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <group>",
		Short: "List a group's field names (never its values)",
		Long: "Print one field name per line, so the output composes with grep, xargs and\n" +
			"--only. Values are never printed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.New()
			if err != nil {
				return err
			}
			names, err := v.GroupFieldNames(args[0])
			if err != nil {
				return err
			}
			for _, n := range names {
				fmt.Fprintln(os.Stdout, n)
			}

			warnf("%d field(s) in group %q\n", len(names), args[0])
			return nil
		},
	}
}
