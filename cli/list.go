package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List vault entries (names and metadata only, never values)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.New()
			if err != nil {
				return err
			}
			names, err := v.List()
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tENV / INFO")
			for _, name := range names {
				typ, info := describe(cfg, name)
				fmt.Fprintf(tw, "%s\t%s\t%s\n", name, typ, info)
			}
			return tw.Flush()
		},
	}
}

// describe returns the entry type and display info using only metadata, so
// listing never reads secret values from the keychain.
func describe(cfg *vault.Config, name string) (typ, info string) {
	if g, ok := cfg.Groups[name]; ok {
		info = g.Description
		if info == "" {
			info = "-"
		}
		return "group", info
	}
	info = cfg.EnvNameForSecret(name)
	if m, ok := cfg.Secrets[name]; ok && m.Description != "" {
		info = info + "  (" + m.Description + ")"
	}
	return "secret", info
}
