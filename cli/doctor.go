package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// defaultTemplateName is compared against a group when --template is not given.
const defaultTemplateName = ".env.example"

func newDoctorCmd() *cobra.Command {
	var template string

	cmd := &cobra.Command{
		Use:   "doctor [entry]",
		Short: "Check a group against an .env template and report drift",
		Long: "Compare a group's field names with a committed template such as\n" +
			defaultTemplateName + ", which should list every variable the project needs with\n" +
			"dummy values. This catches a variable that was needed by the app but never\n" +
			"stored, and one that is stored but no longer used.\n\n" +
			"Missing keys are an error (non-zero exit); extra keys are a warning. The\n" +
			"entry defaults to the one named by " + vault.ProjectFileName + ".",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry := ""
			if len(args) == 1 {
				entry = args[0]
			}

			projPath, proj, err := loadProject()
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
				if err := requireTrustedProject(projPath, proj); err != nil {
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
				tmplPath = filepath.Join(base, defaultTemplateName)
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

			v, err := vault.New()
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

			missing, extra := diffKeys(e.Fields, tmpl)
			isMissing := make(map[string]bool, len(missing))
			for _, k := range missing {
				isMissing[k] = true
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tSTATUS")
			for _, k := range sortedKeys(tmpl) {
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

			warnf("%s: %d/%d template key(s) present", entry, len(tmpl)-len(missing), len(tmpl))
			if len(extra) > 0 {
				warnf("; %d not in the template: %s", len(extra), strings.Join(extra, ", "))
			}
			warnf("\n")

			// Metadata drift is why list can report a group as a secret.
			if cfg, _, cerr := loadConfig(); cerr == nil {
				if _, ok := cfg.Groups[entry]; !ok {
					warnf("warning: %s has no group metadata; \"sanduk list\" reports it as a secret\n", entry)
				}
			}

			if len(missing) > 0 {
				return fmt.Errorf("%d key(s) missing from group %q: %s",
					len(missing), entry, strings.Join(missing, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&template, "template", "",
		"template file to compare against (default "+defaultTemplateName+")")
	cmd.ValidArgsFunction = completeEntryNames
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
