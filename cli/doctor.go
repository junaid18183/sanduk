package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// defaultTemplateName is compared against a group when --template is not given.
const defaultTemplateName = ".env.example"

// templateNames are the committed key lists a repository is checked against,
// in the order they are looked for. Both spellings are common and a project
// that uses the second one should not need --template on every run.
var templateNames = []string{defaultTemplateName, ".env.template"}

// findTemplate returns the first template present in dir, or "" if none is.
func findTemplate(dir string) string {
	for _, n := range templateNames {
		path := filepath.Join(dir, n)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// exitDrift is doctor's exit status when the group is missing template keys.
const exitDrift = 3

func newDoctorCmd(a *app) *cobra.Command {
	var (
		template  string
		metadata  bool
		links     bool
		expiry    bool
		showChain bool
		within    int
		fix       bool
	)

	cmd := &cobra.Command{
		Use:   "doctor [entry]",
		Short: "Check a group against an .env template and report drift",
		Long: "Compare a group's field names with a committed template such as\n" +
			defaultTemplateName + ", which should list every variable the project needs with\n" +
			"dummy values. This catches a variable that was needed by the app but never\n" +
			"stored, and one that is stored but no longer used.\n\n" +
			"Missing keys exit with status 3; extra keys are a warning. The entry\n" +
			"defaults to the one named by " + vault.ProjectFileName + ", once approved.\n\n" +
			"--metadata instead compares the vault with the metadata config: entries\n" +
			"with no metadata (list cannot tell their type) and metadata for entries\n" +
			"that no longer exist. --fix repairs both; recording an untracked entry's\n" +
			"type reads it, which may prompt on some backends.\n\n" +
			"--links checks every group link: a link whose target was removed, or whose\n" +
			"target is no longer a flat secret, would fail the next exec. It reads each\n" +
			"entry, so it may prompt on a presence-gated backend. --fix does not repair\n" +
			"a dangling link — only \"sanduk group link\" or \"unlink\" can decide that — it\n" +
			"rewrites the linked_by hints list uses to group related entries.\n\n" +
			"--expiry lists the entries carrying a rotation deadline, soonest first, and\n" +
			"exits 3 if any has passed or falls due within --within days (default 14).\n" +
			"It reads metadata only: no value is decrypted, so it is safe in CI.\n\n" +
			"--chain reports the " + vault.ProjectFileName + " files that apply here, outermost\n" +
			"first, and which variable each one provides. A file above this directory is\n" +
			"not in the repository, so what it contributes is there on this machine and\n" +
			"nowhere else: the report marks those variables, because a clone or a CI job\n" +
			"running the same command will not have them. Set inherit = false to depend\n" +
			"on this directory's file alone. It exits 3 if a layer is not approved.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if n := boolCount(metadata, links, expiry, showChain); n > 1 {
				return fmt.Errorf("--metadata, --links, --expiry and --chain are separate checks; " +
					"run one at a time")
			}
			if showChain {
				if len(args) > 0 {
					return fmt.Errorf("--chain checks this directory; it takes no entry")
				}
				if fix {
					return fmt.Errorf("--fix applies to --metadata; approve a layer with \"sanduk allow\"")
				}
				return a.doctorChain(cmd)
			}
			if expiry {
				if len(args) > 0 {
					return fmt.Errorf("--expiry checks the whole vault; it takes no entry")
				}
				if fix {
					return fmt.Errorf("--fix applies to --metadata; move a deadline with " +
						"\"sanduk rotate <name> --expires\"")
				}
				return a.doctorExpiry(cmd, within)
			}
			if metadata {
				if len(args) > 0 {
					return fmt.Errorf("--metadata checks the whole vault; it takes no entry")
				}
				return a.doctorMetadata(cmd, fix)
			}
			if links {
				if len(args) > 0 {
					return fmt.Errorf("--links checks the whole vault; it takes no entry")
				}
				return a.doctorLinks(cmd, fix)
			}
			if fix {
				return fmt.Errorf("--fix applies to --metadata")
			}
			entry := ""
			if len(args) == 1 {
				entry = args[0]
			}

			projPath, proj, err := vault.LoadProject(".")
			if err != nil {
				return err
			}
			if entry == "" {
				if proj == nil {
					return fmt.Errorf("no entry given and no %s found; use \"sanduk doctor <entry>\"",
						vault.ProjectFileName)
				}
				// The same approval exec and export require: an unapproved
				// repository must not choose which entry sanduk reads.
				if err := a.requireTrustedProject(proj); err != nil {
					return err
				}
				entry = proj.Entry
			}

			base := "."
			if projPath != "" {
				base = filepath.Dir(projPath)
			}
			tmplPath := template
			if tmplPath == "" {
				if tmplPath = findTemplate(base); tmplPath == "" {
					return fmt.Errorf("no template in %s (looked for %s); create one or pass --template",
						base, strings.Join(templateNames, ", "))
				}
			}
			raw, err := os.ReadFile(tmplPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no template at %s; create it or pass --template", tmplPath)
				}
				return fmt.Errorf("reading template %s: %w", tmplPath, err)
			}
			tmpl, err := vault.ParseDotenv(raw)
			if err != nil {
				return fmt.Errorf("%s: %w", tmplPath, err)
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			e, err := v.Get(entry)
			if err != nil {
				return err
			}
			if e.Type != vault.TypeGroup {
				return fmt.Errorf("%q is a flat secret; doctor compares groups against a template", entry)
			}

			// A linked field is as present as a stored one; comparing only
			// e.Fields would report every link as missing.
			present := make(map[string]string, len(e.Fields)+len(e.Links))
			for k, val := range e.Fields {
				present[k] = val
			}
			for k, target := range e.Links {
				present[k] = target
			}
			missing, extra := diffKeys(present, tmpl)
			isMissing := make(map[string]bool, len(missing))
			for _, k := range missing {
				isMissing[k] = true
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tSTATUS")
			for _, k := range sortedMapKeys(tmpl) {
				status := "ok"
				if isMissing[k] {
					status = "missing"
				}
				fmt.Fprintf(tw, "%s\t%s\n", k, status)
			}
			for _, k := range extra {
				fmt.Fprintf(tw, "%s\textra\n", k)
			}
			if err := tw.Flush(); err != nil {
				return err
			}

			a.warnf("%s: %d/%d template key(s) present", entry, len(tmpl)-len(missing), len(tmpl))
			if len(extra) > 0 {
				a.warnf("; %d not in the template: %s", len(extra), strings.Join(extra, ", "))
			}
			a.warnf("\n")

			// Metadata drift is why list can report a group as a secret.
			if cfg, cerr := a.loadConfig(); cerr == nil {
				if _, ok := cfg.Groups[entry]; !ok {
					a.warnf("warning: %s has no group metadata (fix: sanduk doctor --metadata --fix)\n", entry)
				}
			}

			if len(missing) > 0 {
				// A distinct status, so a script can tell "drift found" (3)
				// from "could not check" (1).
				return &vault.ExitError{Code: exitDrift, Err: fmt.Errorf("%d key(s) missing from group %q: %s",
					len(missing), entry, strings.Join(missing, ", "))}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&metadata, "metadata", false, "check the metadata config against the vault instead")
	cmd.Flags().BoolVar(&showChain, "chain", false,
		"report the "+vault.ProjectFileName+" files that apply here and what each contributes")
	cmd.Flags().BoolVar(&links, "links", false, "check every group link resolves to a flat secret")
	cmd.Flags().BoolVar(&expiry, "expiry", false, "report entries whose rotation deadline has passed or is near")
	cmd.Flags().IntVar(&within, "within", 14, "with --expiry, how many days ahead counts as due")
	cmd.Flags().BoolVar(&fix, "fix", false, "with --metadata, repair the drift it finds")
	cmd.Flags().StringVar(&template, "template", "",
		"template file to compare against (default "+defaultTemplateName+")")
	cmd.ValidArgsFunction = a.completeEntryNames
	return cmd
}

// diffKeys compares a group's fields against a template's keys, returning the
// template keys that are absent and the stored keys the template does not list.
func diffKeys(present, template map[string]string) (missing, extra []string) {
	for k := range template {
		if _, ok := present[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range present {
		if _, ok := template[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// doctorMetadata reports (and with fix, repairs) drift between the backend and
// the metadata config. Only names are listed; values are read only by --fix,
// to learn an untracked entry's type.
func (a *app) doctorMetadata(cmd *cobra.Command, fix bool) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	keys, err := v.List()
	if err != nil {
		return err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	untracked, orphaned := vault.MetadataDrift(keys, cfg)

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS")
	for _, n := range untracked {
		fmt.Fprintf(tw, "%s\tno metadata\n", n)
	}
	for _, n := range orphaned {
		fmt.Fprintf(tw, "%s\tnot in vault\n", n)
	}
	// A group whose directory is gone still works — it is global again —
	// so this is reported next to the drift, not treated as drift. --fix
	// leaves it alone: the directory may be on a volume that is not
	// mounted, and re-approving in the new place is the honest repair.
	stale := cfg.StaleLocations()
	for _, n := range stale {
		fmt.Fprintf(tw, "%s\tlocation %s is gone; the group is global until "+
			"\"sanduk allow\" is run in its new place\n", n, cfg.Groups[n].Location)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(untracked)+len(orphaned) == 0 {
		a.warnf("metadata matches the vault (%d entries)\n", len(keys))
		if len(stale) > 0 {
			a.warnf("%d group location(s) no longer exist\n", len(stale))
		}
		return nil
	}
	if !fix {
		return &vault.ExitError{Code: exitDrift, Err: fmt.Errorf(
			"%d entry/entries without metadata, %d stale record(s); run \"sanduk doctor --metadata --fix\"",
			len(untracked), len(orphaned))}
	}

	types := make(map[string]string, len(untracked))
	for _, n := range untracked {
		e, err := v.Get(n)
		if err != nil {
			return err
		}
		types[n] = e.Type
	}
	if err := a.updateConfig(func(c *vault.Config) {
		for _, n := range orphaned {
			c.Forget(n)
		}
		for n, typ := range types {
			switch typ {
			case vault.TypeGroup:
				c.RecordGroup(n, "")
			case vault.TypeFile:
				c.RecordFile(n, "", "")
			default:
				c.RecordSecret(n, "", "")
			}
		}
	}); err != nil {
		return err
	}
	a.warnf("fixed: recorded %d entry/entries, removed %d stale record(s)\n", len(untracked), len(orphaned))
	return nil
}

// doctorLinks reports group links whose target has been removed or is no longer
// a flat secret. Such a link fails closed at resolve time; this finds it before
// a deploy does.
func (a *app) doctorLinks(cmd *cobra.Command, fix bool) error {
	v, err := a.openVault()
	if err != nil {
		return err
	}
	names, err := v.List()
	if err != nil {
		return err
	}

	// Every entry is read exactly once. The type and the links are both kept
	// from that one read — a second read of the same entry is a second
	// prompt on a dedicated keychain or keychain-biometric — and the values
	// are not, so nothing secret is held while the report is built.
	types := make(map[string]string, len(names))
	links := make(map[string]map[string]string, len(names))
	for _, n := range names {
		e, err := v.Get(n)
		if err != nil {
			return err
		}
		types[n] = e.Type
		if e.Type == vault.TypeGroup {
			links[n] = e.Links
		}
	}

	type linkRef struct{ group, key, target, status string }
	var rows []linkRef
	dangling := 0
	// The link graph as the backend actually holds it, which is what the
	// config's display hints are supposed to mirror.
	byTarget := map[string][]string{}
	for _, n := range names {
		if types[n] != vault.TypeGroup {
			continue
		}
		for _, k := range sortedMapKeys(links[n]) {
			target := links[n][k]
			status := "ok"
			switch typ, ok := types[target]; {
			case !ok:
				status = "missing"
			case typ != vault.TypeSecret:
				status = "not a flat secret"
			}
			if status != "ok" {
				dangling++
			} else {
				byTarget[target] = append(byTarget[target], n)
			}
			rows = append(rows, linkRef{group: n, key: k, target: target, status: status})
		}
	}

	if fix {
		if err := a.updateConfig(func(c *vault.Config) { c.SetLinkedBy(byTarget) }); err != nil {
			return err
		}
		a.warnf("fixed: rebuilt the linked_by hints for %d secret(s)\n", len(byTarget))
	}

	if len(rows) == 0 {
		a.warnf("no group links to check\n")
		return nil
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "GROUP\tKEY\tTARGET\tSTATUS")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.group, r.key, r.target, r.status)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if dangling == 0 {
		a.warnf("%d link(s) resolve\n", len(rows))
		return nil
	}
	return &vault.ExitError{Code: exitDrift, Err: fmt.Errorf("%d dangling link(s) of %d", dangling, len(rows))}
}

// doctorChain reports the project chain that applies here: which layers are
// approved, which variable each provides, and which of them come from above
// this directory.
//
// That last column is the honest cost of inheritance. A file above the
// repository root is not in the clone, so "sanduk exec -- ./deploy" can work
// on this machine and fail in CI with no local sign of why. Naming those
// variables is better than discovering them in a red build.
func (a *app) doctorChain(cmd *cobra.Command) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	layers, why := a.projectChain(cfg)
	if why != nil {
		return why
	}
	if len(layers) == 0 {
		return fmt.Errorf("no %s in this directory or its parents", vault.ProjectFileName)
	}
	unapproved := a.warnUntrusted(layers)

	v, err := a.openVault()
	if err != nil {
		return err
	}
	block, err := a.projectBlockFor(v, cfg, layers, false)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	tw := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "ENTRY\tFROM\tSTATUS")
	paths := layerPaths(layers)
	for i, l := range layers {
		status := "applies here"
		switch {
		case !l.Trusted:
			status = "NOT APPROVED (contributes nothing)"
		case l.Dir != block.Dir:
			status = "outside this directory"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", l.Entry, paths[i], status)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// Which layer each winning variable came from, so the ones that do not
	// travel with the repository can be named.
	entryDir := map[string]string{}
	for _, l := range layers {
		if !l.Trusted {
			continue
		}
		for _, e := range l.entries() {
			entryDir[e] = l.Dir
		}
	}
	var outside []string
	for _, k := range block.Keys {
		if entryDir[k.From] != block.Dir {
			outside = append(outside, k.Name)
		}
	}

	if len(outside) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "%d variable(s) come from outside %s, so a clone or CI will not have them:\n",
			len(outside), shortPath(block.Dir))
		for _, k := range outside {
			fmt.Fprintf(out, "  %s\n", k)
		}
	}

	if unapproved > 0 {
		return &vault.ExitError{
			Code: exitDrift,
			Err:  fmt.Errorf("%d layer(s) of %d are not approved", unapproved, len(layers)),
		}
	}
	a.warnf("%d layer(s) resolve, %d variable(s)\n", len(layers), len(block.Keys))
	return nil
}

// boolCount counts the set flags among several mutually exclusive modes.
func boolCount(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// doctorExpiry reports rotation deadlines, soonest first. It reads the metadata
// config only — never a secret value — so it is safe to run unattended, on a
// presence-gated backend, and in CI.
func (a *app) doctorExpiry(cmd *cobra.Command, within int) error {
	if within < 0 {
		return fmt.Errorf("--within must not be negative")
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	due, err := cfg.Expiries(time.Now())
	if err != nil {
		return err
	}
	if len(due) == 0 {
		a.warnf("no entry carries an expiry date (set one with --expires)\n")
		return nil
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tEXPIRES\tSTATUS")
	overdue, soon := 0, 0
	for _, e := range due {
		var status string
		switch {
		case e.Expired():
			status = fmt.Sprintf("expired %d day(s) ago", -e.Days)
			overdue++
		case e.Days <= within:
			status = fmt.Sprintf("due in %d day(s)", e.Days)
			soon++
		default:
			status = fmt.Sprintf("ok (%d days)", e.Days)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, e.Date, status)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if overdue+soon == 0 {
		a.warnf("%d entry/entries with a deadline, none due within %d day(s)\n", len(due), within)
		return nil
	}
	return &vault.ExitError{Code: exitDrift, Err: fmt.Errorf(
		"%d expired, %d due within %d day(s)", overdue, soon, within)}
}
