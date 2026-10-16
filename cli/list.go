package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var (
		asJSON    bool
		namesOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List vault entries (names and metadata only, never values)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := openVault()
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

			if namesOnly {
				for _, n := range names {
					fmt.Fprintln(cmd.OutOrStdout(), n)
				}
				return nil
			}

			if asJSON {
				type row struct {
					Name string `json:"name"`
					Type string `json:"type"`
					Info string `json:"info"`
				}
				rows := make([]row, 0, len(names))
				for _, name := range names {
					typ, info := describe(cfg, name)
					rows = append(rows, row{Name: name, Type: typ, Info: info})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tENV / INFO")
			for _, name := range names {
				typ, info := describe(cfg, name)
				fmt.Fprintf(tw, "%s\t%s\t%s\n", name, typ, info)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a JSON array of entries")
	cmd.Flags().BoolVar(&namesOnly, "names", false, "print entry names only, one per line")
	cmd.MarkFlagsMutuallyExclusive("json", "names")
	return cmd
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
