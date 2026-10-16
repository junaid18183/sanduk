package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var (
		force  bool
		field  string
		all    bool
		reveal bool
	)

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Print a secret value",
		Long: "Print a secret value. To reduce accidental leakage, sanduk refuses to\n" +
			"print a secret when stdout is not a terminal unless --force is given.\n\n" +
			"For a group, name the field you want with --field (redacted unless --reveal\n" +
			"is also given), or pass --all to print every field as JSON. Reading a single\n" +
			"field is the least-privilege way to review one secret.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !vault.IsTerminal(os.Stdout) {
				return fmt.Errorf("refusing to print a secret to a non-terminal; use --force to override")
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			e, err := v.Get(args[0])
			if err != nil {
				return err
			}

			switch e.Type {
			case vault.TypeSecret:
				if field != "" || all {
					return fmt.Errorf("%q is a flat secret; --field and --all apply to groups", args[0])
				}
				fmt.Fprintln(os.Stdout, e.Value)

			case vault.TypeGroup:
				if field != "" {
					val, ok := e.Fields[field]
					if !ok {
						return fmt.Errorf("%q has no field %q (fields: %s)",
							args[0], field, strings.Join(sortedMapKeys(e.Fields), ", "))
					}
					if reveal {
						fmt.Fprintln(os.Stdout, val)
					} else {
						fmt.Fprintln(os.Stdout, vault.Redact(val))
					}
					return nil
				}
				if !all {
					return fmt.Errorf("%q is a group with %d field(s); pass --field KEY to print one, or --all for every field",
						args[0], len(e.Fields))
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(e.Fields)

			default:
				return fmt.Errorf("unknown entry type %q", e.Type)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "allow printing to a non-terminal")
	cmd.Flags().StringVar(&field, "field", "", "print only this field of a group")
	cmd.Flags().BoolVar(&all, "all", false, "print every field of a group as JSON")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "print the full value of --field instead of a redacted one")
	cmd.MarkFlagsMutuallyExclusive("field", "all")
	cmd.ValidArgsFunction = completeEntryNames
	return cmd
}
