package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newListCmd(a *app) *cobra.Command {
	var (
		asJSON    bool
		namesOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List vault entries (names and metadata only, never values)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := a.openVault()
			if err != nil {
				return err
			}
			names, err := v.List()
			if err != nil {
				return err
			}
			cfg, err := a.loadConfig()
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
		return "group", info + expiryNote(g.Expires)
	}
	if m, ok := cfg.Files[name]; ok {
		return "file", describeSecretMeta(cfg, name, m)
	}
	m, ok := cfg.Secrets[name]
	if !ok {
		// Without metadata the type cannot be known without reading the value.
		return "unknown", "no metadata; run \"sanduk doctor --metadata --fix\""
	}
	return "secret", describeSecretMeta(cfg, name, m)
}

// describeSecretMeta renders the INFO column for a flat or file secret: the
// variable it is exposed as, then any description and deadline.
func describeSecretMeta(cfg *vault.Config, name string, m vault.SecretMeta) string {
	info := cfg.EnvNameForSecret(name)
	if m.Description != "" {
		info = info + "  (" + m.Description + ")"
	}
	return info + expiryNote(m.Expires)
}

// expiryNote renders a deadline for the list's INFO column. An overdue entry
// says so rather than printing a date the reader has to compare against today.
func expiryNote(date string) string {
	if date == "" {
		return ""
	}
	t, err := vault.ParseExpiry(date)
	if err != nil {
		return "  [expires " + date + "?]"
	}
	if daysUntil(t) < 0 {
		return "  [EXPIRED " + date + "]"
	}
	return "  [expires " + date + "]"
}

// daysUntil is whole days from today (UTC) until t.
func daysUntil(t time.Time) int {
	return int(t.Sub(time.Now().UTC().Truncate(24*time.Hour)).Hours() / 24)
}
