package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newGroupListCmd(a *app) *cobra.Command {
	var (
		asJSON    bool
		namesOnly bool
		withKeys  bool
		here      bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the groups in the vault and the directory each belongs to",
		Long: "List every group, with the directory it belongs to and what it is for.\n\n" +
			"A group created inside a repository is recorded against that directory, so\n" +
			"this answers \"which repository does this group serve\" without opening a\n" +
			"single entry: the table is built from metadata alone.\n\n" +
			"--keys adds the variables each group provides. That opens every group in\n" +
			"turn, which is slower and may prompt once per group on a presence-gated\n" +
			"backend; values are still never read into the output.\n" +
			"--here lists only the group the working directory belongs to.",
		Args: cobra.NoArgs,
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

			layers, why := a.projectChain(cfg)
			if why != nil {
				a.warnf("note: %v\n", why)
			}
			a.warnUntrusted(layers)
			// Every layer of the chain applies here, so every one of them is
			// a group this directory belongs to.
			current := map[string]bool{}
			for _, l := range layers {
				if l.Trusted {
					current[l.Entry] = true
				}
			}
			// Only the locations that still exist; the rest are reported as
			// missing rather than shown as if they meant something.
			locations := cfg.GroupLocations()

			// The backend decides what exists; the config only says which of
			// those entries is a group. A group whose metadata was lost is
			// reported rather than silently dropped.
			var groups, untracked []string
			for _, n := range names {
				if _, ok := cfg.Groups[n]; ok {
					if !here || current[n] {
						groups = append(groups, n)
					}
					continue
				}
				if _, s := cfg.Secrets[n]; !s {
					if _, f := cfg.Files[n]; !f {
						untracked = append(untracked, n)
					}
				}
			}

			if namesOnly {
				for _, n := range groups {
					fmt.Fprintln(cmd.OutOrStdout(), n)
				}
				return nil
			}

			rows := make([]groupRow, 0, len(groups))
			for _, n := range groups {
				g := cfg.Groups[n]
				r := groupRow{
					Name:        n,
					Location:    g.Location,
					Description: g.Description,
					Expires:     g.Expires,
					Current:     current[n],
					Present:     locations[n] != "",
				}
				if withKeys {
					vars, err := v.Variables(n, cfg)
					if err != nil {
						return err
					}
					r.Keys = vars.Names
				}
				rows = append(rows, r)
			}

			sortGroupRows(rows)

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(rows); err != nil {
					return err
				}
			} else if err := printGroupTable(cmd, rows, withKeys); err != nil {
				return err
			}

			if len(rows) == 0 {
				a.warnf("no groups yet — create one with \"sanduk add-group <name>\"\n")
			}
			if len(current) > 0 && !namesOnly && !asJSON {
				a.warnf("* the group(s) this directory belongs to\n")
			}
			if len(untracked) > 0 {
				a.warnf("%d entry/entries have no metadata and may be groups; "+
					"run \"sanduk doctor --metadata --fix\"\n", len(untracked))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a JSON array of groups")
	cmd.Flags().BoolVar(&namesOnly, "names", false, "print group names only, one per line")
	cmd.Flags().BoolVar(&withKeys, "keys", false, "also list the variables each group provides (opens every group)")
	cmd.Flags().BoolVar(&here, "here", false, "list only the group this directory belongs to")
	cmd.MarkFlagsMutuallyExclusive("json", "names")
	return cmd
}

// sortGroupRows puts the rows the reader is standing in front of first: the
// groups that apply here, then the ones above and below this directory, then
// everything else alphabetically.
//
// A vault that is used holds a group per repository, and a flat alphabetical
// list buries the two or three that matter where you are among a machine's
// worth of others.
func sortGroupRows(rows []groupRow) {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	rank := func(r groupRow) int {
		switch {
		case r.Current:
			return 0
		case r.Present && withinDir(r.Location, wd):
			return 1 // a directory this one is inside
		case r.Present && wd != "" && withinDir(wd, r.Location):
			return 2 // a repository underneath this one
		default:
			return 3
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rank(rows[i]), rank(rows[j])
		if ri != rj {
			return ri < rj
		}
		// Within a rank, outer directories first: a chain reads top down.
		if ri < 3 && rows[i].Location != rows[j].Location {
			return len(rows[i].Location) < len(rows[j].Location)
		}
		return rows[i].Name < rows[j].Name
	})
}

// groupRow is one row of "group list", and its JSON shape.
type groupRow struct {
	Name        string `json:"name"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	Expires     string `json:"expires,omitempty"`
	Current     bool   `json:"current,omitempty"`
	// Present is false when Location no longer exists, which makes the group
	// global again.
	Present bool     `json:"present,omitempty"`
	Keys    []string `json:"keys,omitempty"`
}

func printGroupTable(cmd *cobra.Command, rows []groupRow, withKeys bool) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	header := "NAME\tLOCATION\tINFO"
	if withKeys {
		header = "NAME\tLOCATION\tKEYS\tINFO"
	}
	fmt.Fprintln(tw, header)
	for _, r := range rows {
		name := r.Name
		if r.Current {
			// The group you are standing in is the one row you are most
			// likely looking for.
			name += " *"
		}
		info := r.Description
		if info == "" {
			info = "-"
		}
		info += expiryNote(r.Expires)
		loc := shortPath(r.Location)
		switch {
		case loc == "":
			loc = "-"
		case !r.Present:
			// The directory is gone, so the group is global again. Say so
			// rather than printing a path that no longer means anything.
			loc += "  (missing)"
		}
		if withKeys {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, loc, joinComma(r.Keys), info)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", name, loc, info)
	}
	return tw.Flush()
}
