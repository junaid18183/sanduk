package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGetCmd(a *app) *cobra.Command {
	var (
		force  bool
		field  string
		all    bool
		reveal bool
	)

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Inspect a secret (redacted unless --reveal)",
		Long: "Inspect a stored value. get is for a person at a terminal: values are\n" +
			"redacted to their last four characters unless --reveal is given, and sanduk\n" +
			"refuses to print to a non-terminal unless --force is given. Scripts should\n" +
			"use export, which is the interface for machines.\n\n" +
			"For a group, name the field you want with --field, or pass --all to print\n" +
			"every field as JSON. Reading a single field is the least-privilege way to\n" +
			"review one secret.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := requireTerminal(out, force); err != nil {
				return err
			}

			v, err := a.openVault()
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
				fmt.Fprintln(out, revealOrRedact(e.Value, reveal))

			case vault.TypeGroup:
				if field != "" {
					return printGroupField(out, args[0], e, field, reveal)
				}
				if !all {
					return fmt.Errorf("%q is a group with %d field(s); pass --field KEY to print one, or --all for every field",
						args[0], len(e.Fields))
				}
				fields := make(map[string]string, len(e.Fields))
				for k, val := range e.Fields {
					fields[k] = revealOrRedact(val, reveal)
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(fields)

			default:
				return fmt.Errorf("unknown entry type %q", e.Type)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "allow printing to a non-terminal")
	cmd.Flags().StringVar(&field, "field", "", "print only this field of a group")
	cmd.Flags().BoolVar(&all, "all", false, "print every field of a group as JSON")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "print full values instead of redacted ones")
	cmd.MarkFlagsMutuallyExclusive("field", "all")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

// printGroupField prints one field of group entry e, redacted unless reveal.
func printGroupField(w io.Writer, name string, e *vault.Entry, field string, reveal bool) error {
	val, ok := e.Fields[field]
	if !ok {
		return fmt.Errorf("%q has no field %q (fields: %s)",
			name, field, strings.Join(sortedMapKeys(e.Fields), ", "))
	}
	_, err := fmt.Fprintln(w, revealOrRedact(val, reveal))
	return err
}

func revealOrRedact(val string, reveal bool) string {
	if reveal {
		return val
	}
	return redact(val)
}
