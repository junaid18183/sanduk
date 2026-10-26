package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
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

			layers, why := a.projectChain(cfg)
			if why != nil {
				a.warnf("note: %v\n", why)
			}
			unusable := a.warnUntrusted(layers)
			if (why != nil || unusable > 0) && !anyTrusted(layers) {
				a.warnf("listing global entries only\n")
			}
			// A layer's directory is what claims its entry, whether or not
			// the entry is a group with a location recorded for it.
			projects := cfg.GroupLocations()
			for _, l := range layers {
				if l.Trusted {
					projects[l.Entry] = l.Dir
				}
			}
			entries := vault.ScopeEntries(names, cfg, projects, currentEntries(layers))

			// The project block is the point of the default view, so it is
			// built whenever the working directory has a chain in scope.
			var block *projectBlock
			if anyTrusted(layers) && (scope == "" || scope == vault.ScopeProject) {
				block, err = a.projectBlockFor(v, cfg, layers, noKeys)
				if err != nil {
					return err
				}
			}

			var shown []vault.Scoped
			hidden := map[string]int{}
			for _, e := range entries {
				if visibleInScope(e, scope, block != nil) {
					shown = append(shown, e)
					continue
				}
				// An entry the block above already shows is not hidden.
				if block != nil && e.Current {
					continue
				}
				hidden[e.Scope]++
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

// visibleInScope reports whether a classified entry belongs in the table.
//
// The default view answers "what applies where I am standing". Inside a
// project that is the chain, shown above the table, and the table itself is
// empty: the vault's global entries are a different question, and printing
// them underneath buries the three keys the directory actually uses. Outside
// any project there is no chain to show, so the globals are the answer.
//
// Nothing is lost silently either way — reportHidden always says how many
// entries the filter left out, and --all or --scope global shows them.
func visibleInScope(e vault.Scoped, scope string, inProject bool) bool {
	switch scope {
	case scopeAll:
		return true
	case "":
		return e.Scope == vault.ScopeGlobal && !inProject
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

// projectBlock is the chain of project layers that claims the working
// directory, expanded into the variables they provide. Values are never read:
// keys carries names, link targets and which entry each name came from.
type projectBlock struct {
	Layers []projectLayer
	Dir    string
	Keys   []projectKey
}

// projectKey is one variable the chain provides. From names the entry whose
// value wins, LinkedTo the flat secret that entry reads it from, and Shadows
// the entries an earlier layer would have provided it from.
type projectKey struct {
	Name     string   `json:"name"`
	From     string   `json:"from"`
	LinkedTo string   `json:"linked_to,omitempty"`
	Shadows  []string `json:"shadows,omitempty"`
}

// projectBlockFor reads the variable names each layer provides, and works out
// which layer wins each one.
//
// This is the only read a listing makes. A single-layer project opens one
// entry, as before; a chain opens the entries it resolves, which on a
// presence-gated backend is one prompt per distinct entry — hence the cache,
// and hence --no-keys, which skips the reads altogether. No value is read,
// and an untrusted layer is never opened: it injects nothing, so reporting
// its keys would describe a run that cannot happen.
func (a *app) projectBlockFor(v *vault.Vault, cfg *vault.Config, layers []projectLayer, noKeys bool) (*projectBlock, error) {
	b := &projectBlock{Layers: layers, Dir: innermostDir(layers)}
	if noKeys {
		return b, nil
	}

	seen := map[string]int{} // variable name -> index in b.Keys
	cache := map[string]*vault.Variables{}
	for _, l := range layers {
		if !l.Trusted {
			continue
		}
		for _, entry := range l.entries() {
			vars, ok := cache[entry]
			if !ok {
				got, err := v.Variables(entry, cfg)
				if err != nil {
					// A project file naming an entry that is not stored yet
					// is a normal state in a fresh clone; say so instead of
					// failing the listing.
					a.warnf("note: %s names entry %q: %v\n",
						filepath.Join(l.Dir, vault.ProjectFileName), entry, err)
					cache[entry] = &vault.Variables{}
					continue
				}
				vars, cache[entry] = got, got
			}
			for _, n := range vars.Names {
				k := projectKey{Name: n, From: entry, LinkedTo: vars.Links[n]}
				i, dup := seen[n]
				if !dup {
					seen[n] = len(b.Keys)
					b.Keys = append(b.Keys, k)
					continue
				}
				// A nearer layer wins, and says what it displaced: which
				// GITHUB_TOKEN you get is the question a chain raises.
				k.Shadows = append(b.Keys[i].Shadows, b.Keys[i].From)
				b.Keys[i] = k
			}
		}
	}
	sort.Slice(b.Keys, func(i, j int) bool { return b.Keys[i].Name < b.Keys[j].Name })
	return b, nil
}

// anyTrusted reports whether any layer is approved.
//
// A chain of nothing but unapproved files claims nothing: it must not narrow
// the listing, or a repository could hide the rest of the vault from you
// simply by committing a file. The note on stderr says why, and the globals
// are listed as if the file were not there.
func anyTrusted(layers []projectLayer) bool {
	for _, l := range layers {
		if l.Trusted {
			return true
		}
	}
	return false
}

// kindNote renders a layer's type and anything the reader has to know about
// it before trusting what the keys below say.
func kindNote(cfg *vault.Config, l projectLayer) string {
	out := kindOf(cfg, l.Entry)
	switch {
	case !l.Trusted:
		out += "  (not approved; not applied)"
	case l.FromLocation:
		out += "  (your local record, not " + vault.ProjectFileName + ")"
	}
	if len(l.Include) > 0 {
		out += "  (+" + joinComma(l.Include) + ")"
	}
	return out
}

func printListTable(cmd *cobra.Command, cfg *vault.Config, block *projectBlock, shown []vault.Scoped, withScope bool) error {
	out := cmd.OutOrStdout()
	if block != nil {
		if err := printProjectBlock(out, cfg, block); err != nil {
			return err
		}
	}
	if len(shown) == 0 && block != nil {
		return nil
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
	type layerRow struct {
		Dir     string   `json:"dir"`
		Entry   string   `json:"entry"`
		Type    string   `json:"type"`
		Include []string `json:"include,omitempty"`
		Only    []string `json:"only,omitempty"`
		Trusted bool     `json:"trusted"`
		// Location marks a layer that came from your own config rather than
		// from a project file, so a reader can tell what a clone would see.
		Location bool `json:"location,omitempty"`
	}
	type projectRow struct {
		Dir    string       `json:"dir"`
		Layers []layerRow   `json:"layers"`
		Keys   []projectKey `json:"keys,omitempty"`
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
		row := &projectRow{Dir: block.Dir, Layers: []layerRow{}, Keys: block.Keys}
		for _, l := range block.Layers {
			row.Layers = append(row.Layers, layerRow{
				Dir: l.Dir, Entry: l.Entry, Type: kindOf(cfg, l.Entry),
				Include: l.Include, Only: l.Only,
				Trusted: l.Trusted, Location: l.FromLocation,
			})
		}
		doc.Project = row
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

// printProjectBlock renders the chain and the variables it provides.
//
// One layer is the common case and prints as it always has. More than one and
// the layers are listed in resolution order, with a FROM column on the keys:
// with two directories both providing GITHUB_TOKEN, the only thing worth
// printing is which one you actually get.
func printProjectBlock(out io.Writer, cfg *vault.Config, block *projectBlock) error {
	single := len(block.Layers) == 1
	if single {
		l := block.Layers[0]
		fmt.Fprintf(out, "PROJECT  %s (%s)  %s\n", l.Entry, kindNote(cfg, l), shortPath(l.Dir))
	} else {
		fmt.Fprintf(out, "PROJECT  %s\n", shortPath(block.Dir))
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ENTRY\tFROM\tTYPE")
		paths := layerPaths(block.Layers)
		for i, l := range block.Layers {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", l.Entry, paths[i], kindNote(cfg, l))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		fmt.Fprintln(out)
	}

	if len(block.Keys) > 0 {
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		if single {
			fmt.Fprintln(tw, "KEY\tSOURCE")
		} else {
			fmt.Fprintln(tw, "KEY\tFROM\tSOURCE")
		}
		for _, k := range block.Keys {
			if single {
				fmt.Fprintf(tw, "%s\t%s\n", k.Name, keySource(k))
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", k.Name, k.From, keySource(k))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	fmt.Fprintln(out)
	return nil
}

// keySource renders the SOURCE column of the project block: where the value
// comes from, and what providing it displaced.
func keySource(k projectKey) string {
	out := "stored"
	if k.LinkedTo != "" {
		out = "-> " + k.LinkedTo
	}
	if len(k.Shadows) > 0 {
		out += "  (shadows " + joinComma(k.Shadows) + ")"
	}
	return out
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
