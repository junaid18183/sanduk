package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Print a secret value",
		Long: "Print a secret value. To reduce accidental leakage, sanduk refuses to\n" +
			"print a secret when stdout is not a terminal unless --force is given.",
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
				fmt.Fprintln(os.Stdout, e.Value)
			case vault.TypeGroup:
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
	return cmd
}
