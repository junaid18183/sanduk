package cli

import (
	"fmt"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGetCmd(a *app) *cobra.Command {
	var (
		force  bool
		field  string
		reveal bool
	)

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Inspect a secret (redacted unless --reveal)",
		Long: "Inspect a stored value. get is for a person at a terminal: values are\n" +
			"redacted to their last four characters unless --reveal is given, and sanduk\n" +
			"refuses to print to a non-terminal unless --force is given. Scripts should\n" +
			"use export, which is the interface for machines.\n\n" +
			"For a group, name the field you want with --field. Reading a single field is\n" +
			"the least-privilege way to review one secret; export prints every field.",
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
				if field != "" {
					return fmt.Errorf("%q is a flat secret; --field applies to groups", args[0])
				}
				fmt.Fprintln(out, revealOrRedact(e.Value, reveal))

			case vault.TypeGroup:
				if field != "" {
					// Resolve only the named field, so reviewing one secret
					// does not read the group's other link targets.
					val, linkedTo, err := v.GroupFieldValue(args[0], field)
					if err != nil {
						return err
					}
					if linkedTo != "" {
						a.warnf("%s is linked to secret %q\n", field, linkedTo)
					}
					_, err = fmt.Fprintln(out, revealOrRedact(val, reveal))
					return err
				}
				return fmt.Errorf("%q is a group with %d field(s); pass --field KEY to print one, or use \"sanduk export %s\" for all of them",
					args[0], e.FieldCount(), args[0])

			case vault.TypeFile:
				if field != "" {
					return fmt.Errorf("%q is a file secret; --field applies to groups", args[0])
				}
				// File secrets are opaque bytes, often binary. Printing them
				// to a terminal is never what anyone wants, and get is the
				// terminal-facing command; export --materialize writes them out.
				name := vault.SafeFilename(args[0], vault.FilePayload{Filename: e.Filename})
				fmt.Fprintf(out, "file secret: %d bytes, materialized as %q\n", len(e.Data), name)
				if reveal {
					a.warnf("--reveal does not dump file bytes; use \"sanduk export %s --materialize DIR\"\n", args[0])
				}
				return nil

			default:
				return fmt.Errorf("unknown entry type %q", e.Type)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "allow printing to a non-terminal")
	cmd.Flags().StringVar(&field, "field", "", "print only this field of a group")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "print full values instead of redacted ones")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

func revealOrRedact(val string, reveal bool) string {
	if reveal {
		return val
	}
	return redact(val)
}
