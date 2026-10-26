package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// scopeAll is the --scope value that turns every filter off.
const scopeAll = "all"

func newListCmd(a *app) *cobra.Command {
	var (
		asJSON    bool
		namesOnly bool
		all       bool
		scope     string
		noKeys    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List vault entries (names and metadata only, never values)",
		Long: "List the entries that matter where you are standing.\n\n" +
			"By default only global entries are listed: the ones that stand on their\n" +
			"own. Two kinds are left out, because a vault that is used grows a long\n" +
			"tail of both:\n\n" +
			"  - entries claimed by an approved " + vault.ProjectFileName + " somewhere on this\n" +
			"    machine, which belong to that repository, and\n" +
			"  - flat secrets that a group links to, which are that group's building\n" +
			"    blocks rather than something to reach for directly.\n\n" +
			"Inside a repository whose " + vault.ProjectFileName + " is approved, that project's\n" +
			"entry is shown first, expanded into the variables it provides. Reading\n" +
			"those names opens that one entry (and may prompt on a presence-gated\n" +
			"backend); --no-keys lists the entry without them. No value is ever read\n" +
			"for the table itself, or ever printed.\n\n" +
			"--all, or --scope project|member|global, shows what the default hides.\n" +
			"The count of hidden entries is always reported on stderr.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				scope = scopeAll
			}
			if scope != "" && scope != scopeAll && !vault.ValidScope(scope) {
				return fmt.Errorf("unknown scope %q: use project, member, global or all", scope)
			}

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

			projPath, projEntry, why := a.projectScope(cfg)
			if why != nil {
				a.warnf("note: %v\n", why)
				if projEntry == "" {
					a.warnf("listing global entries only\n")
				}
			}
			entries := vault.ScopeEntries(names, cfg, cfg.GroupLocations(), projEntry)

			var shown []vault.Scoped
			hidden := map[string]int{}
			for _, e := range entries {
				if visibleInScope(e, scope) {
					shown = append(shown, e)
					continue
				}
				hidden[e.Scope]++
			}

			// The project block is the point of the default view, so it is
			// built whenever the current repository has one in scope.
			var block *projectBlock
			if projEntry != "" && (scope == "" || scope == vault.ScopeProject) {
				block, err = a.projectBlockFor(v, cfg, projPath, projEntry, noKeys)
				if err != nil {
					return err
				}
			}

			switch {
			case namesOnly:
				for _, e := range shown {
					fmt.Fprintln(cmd.OutOrStdout(), e.Name)
				}
			case asJSON:
				if err := printListJSON(cmd, cfg, block, shown, hidden); err != nil {
					return err
				}
			default:
				if err := printListTable(cmd, cfg, block, shown, scope == scopeAll); err != nil {
					return err
				}
			}
			a.reportHidden(hidden)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the listing as a JSON object")
	cmd.Flags().BoolVar(&namesOnly, "names", false, "print entry names only, one per line")
	cmd.Flags().BoolVar(&all, "all", false, "list every entry, whatever its scope")
	cmd.Flags().StringVar(&scope, "scope", "", "list one scope only: project, member, global or all")
	cmd.Flags().BoolVar(&noKeys, "no-keys", false,
		"do not open the project entry to list the variables it provides")
	cmd.MarkFlagsMutuallyExclusive("json", "names")
	cmd.MarkFlagsMutuallyExclusive("all", "scope")
	return cmd
}

// visibleInScope reports whether a classified entry belongs in the table. The
// default view is global entries only: the project's own entry is shown above
// the table rather than repeated inside it.
func visibleInScope(e vault.Scoped, scope string) bool {
	switch scope {
	case scopeAll:
		return true
	case "":
		return e.Scope == vault.ScopeGlobal
	default:
		return e.Scope == scope && !e.Current
	}
}

// reportHidden says on stderr how many entries the filter left out, so a
// shorter list never reads as a missing entry.
func (a *app) reportHidden(hidden map[string]int) {
	total := 0
	for _, n := range hidden {
		total += n
	}
	if total == 0 {
		return
	}
	parts := make([]string, 0, len(hidden))
	for _, s := range []string{vault.ScopeMember, vault.ScopeProject, vault.ScopeGlobal} {
		if n := hidden[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	a.warnf("%d entry/entries hidden (%s) — see \"sanduk list --all\"\n",
		total, joinComma(parts))
}

// projectBlock is the repository's own entry, expanded into the variables it
// provides. Values are never read: keys carries names and link targets only.
type projectBlock struct {
	Path  string
	Entry string
	Type  string
	Keys  []projectKey
}

// projectKey is one variable an entry provides. LinkedTo is the flat secret
// the value is read from, empty when the entry stores it.
type projectKey struct {
	Name     string `json:"name"`
	LinkedTo string `json:"linked_to,omitempty"`
}

// projectBlockFor reads the project entry's variable names. It is the one read
// a listing makes, and it is skipped entirely with --no-keys or when the entry
// named by the project file does not exist yet.
func (a *app) projectBlockFor(v *vault.Vault, cfg *vault.Config, path, entry string, noKeys bool) (*projectBlock, error) {
	b := &projectBlock{Path: path, Entry: entry, Type: kindOf(cfg, entry)}
	if noKeys {
		return b, nil
	}
	vars, err := v.Variables(entry, cfg)
	if err != nil {
		// A project file naming an entry that is not stored yet is a normal
		// state in a fresh clone; say so instead of failing the listing.
		a.warnf("note: %s names entry %q: %v\n", vault.ProjectFileName, entry, err)
		return b, nil
	}
	b.Type = vars.Type
	for _, n := range vars.Names {
		b.Keys = append(b.Keys, projectKey{Name: n, LinkedTo: vars.Links[n]})
	}
	return b, nil
}

func printListTable(cmd *cobra.Command, cfg *vault.Config, block *projectBlock, shown []vault.Scoped, withScope bool) error {
	out := cmd.OutOrStdout()
	if block != nil {
		fmt.Fprintf(out, "PROJECT  %s (%s)  %s\n", block.Entry, block.Type, block.Path)
		if len(block.Keys) > 0 {
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tSOURCE")
			for _, k := range block.Keys {
				fmt.Fprintf(tw, "%s\t%s\n", k.Name, keySource(k))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		fmt.Fprintln(out)
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	header := "NAME\tTYPE\tENV / INFO"
	if withScope {
		header = "NAME\tSCOPE\tTYPE\tENV / INFO"
	}
	fmt.Fprintln(tw, header)
	for _, e := range shown {
		typ, info := describe(cfg, e.Name)
		if withScope {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, scopeLabel(e), typ, info)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, typ, info)
	}
	return tw.Flush()
}

func printListJSON(cmd *cobra.Command, cfg *vault.Config, block *projectBlock, shown []vault.Scoped, hidden map[string]int) error {
	type projectRow struct {
		Path  string       `json:"path"`
		Entry string       `json:"entry"`
		Type  string       `json:"type"`
		Keys  []projectKey `json:"keys,omitempty"`
	}
	type entryRow struct {
		Name   string   `json:"name"`
		Type   string   `json:"type"`
		Info   string   `json:"info"`
		Scope  string   `json:"scope"`
		Owners []string `json:"owners,omitempty"`
	}
	doc := struct {
		Project *projectRow    `json:"project,omitempty"`
		Entries []entryRow     `json:"entries"`
		Hidden  map[string]int `json:"hidden,omitempty"`
	}{Entries: []entryRow{}}

	if block != nil {
		doc.Project = &projectRow{
			Path: block.Path, Entry: block.Entry, Type: block.Type, Keys: block.Keys,
		}
	}
	for _, e := range shown {
		typ, info := describe(cfg, e.Name)
		doc.Entries = append(doc.Entries, entryRow{
			Name: e.Name, Type: typ, Info: info, Scope: e.Scope, Owners: e.Owners,
		})
	}
	if len(hidden) > 0 {
		doc.Hidden = hidden
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// keySource renders the SOURCE column of the project block.
func keySource(k projectKey) string {
	if k.LinkedTo != "" {
		return "-> " + k.LinkedTo
	}
	return "stored"
}

// scopeLabel renders an entry's scope, naming what claims it.
func scopeLabel(e vault.Scoped) string {
	switch {
	case e.Current:
		return vault.ScopeProject + " (here)"
	case e.Scope == vault.ScopeMember && len(e.Owners) > 0:
		return vault.ScopeMember + " of " + joinComma(e.Owners)
	default:
		return e.Scope
	}
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// kindOf names an entry's type from metadata alone, for a header printed
// before anything is read.
func kindOf(cfg *vault.Config, name string) string {
	typ, _ := describe(cfg, name)
	return typ
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
