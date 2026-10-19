package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newImportCmd(a *app) *cobra.Command {
	var (
		groupName string
		deleteSrc bool
		dryRun    bool
		replace   bool
		format    string
	)

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import secrets from a .env file or a credential-manager export",
		Long: "Import secrets in bulk. By default each one becomes its own flat secret;\n" +
			"use --group to store them all as one grouped entry.\n\n" +
			"The format is taken from the file extension and can be forced with\n" +
			"--format:\n\n" +
			"  dotenv  KEY=VALUE pairs (the default for any unrecognised extension)\n" +
			"  csv     a credential export; the header picks the name and value\n" +
			"          columns, and a \"fields\" column of \"label: value\" lines\n" +
			"          contributes one entry each\n" +
			"  1pux    a 1Password Unencrypted Export archive\n\n" +
			"For csv and 1pux the entry name comes from the item's title (and the field\n" +
			"label, where there is one), upper-cased with other characters replaced by\n" +
			"underscores. Only secret-bearing fields are read: a login's password and\n" +
			"concealed custom fields. Usernames, URLs, notes and attachments are left\n" +
			"behind.\n\n" +
			"An export file is a plaintext copy of another vault. Review it with\n" +
			"--dry-run first, which prints names only, and consider --delete to remove\n" +
			"it once the import succeeds.\n\n" +
			"An existing entry is not overwritten unless --replace is given.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			if format == "" {
				format = vault.DetectImportFormat(path)
			}
			pairs, err := readImportFile(path, format)
			if err != nil {
				return err
			}
			if len(pairs) == 0 {
				return fmt.Errorf("no secrets found in %s", path)
			}
			keys := vault.SortedNames(pairs)

			if dryRun {
				for _, k := range keys {
					fmt.Fprintln(cmd.OutOrStdout(), k)
				}
				a.warnf("dry run: %d secret(s) would be imported from %s (%s)\n", len(keys), path, format)
				return nil
			}

			// Validate every key before writing anything, so a bad name cannot
			// leave the import half done.
			for _, k := range keys {
				if err := vault.ValidateEnvName(k); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}

			if groupName != "" {
				if err := a.refuseOverwrite(v, groupName, replace, ""); err != nil {
					return err
				}
				if err := v.AddGroup(groupName, pairs); err != nil {
					return err
				}
				if err := a.recordGroupMeta(groupName, ""); err != nil {
					return err
				}
				a.warnf("Imported %d field(s) into group %q\n", len(pairs), groupName)
			} else {
				// Like add-group, never overwrite silently: check every name
				// before writing any, so a refusal leaves nothing half done.
				existing, err := v.List()
				if err != nil {
					return err
				}
				if overwrite := overlap(keys, existing); len(overwrite) > 0 {
					if !replace {
						return fmt.Errorf("%d entry/entries already exist: %s; pass --replace to overwrite them",
							len(overwrite), strings.Join(overwrite, ", "))
					}
					a.warnf("overwriting %d existing entry/entries: %v\n", len(overwrite), overwrite)
				}
				for _, k := range keys {
					if err := v.AddSecret(k, pairs[k]); err != nil {
						return err
					}
				}
				if err := a.updateConfig(func(c *vault.Config) {
					for _, k := range keys {
						c.RecordSecret(k, k, "")
					}
				}); err != nil {
					return err
				}
				a.warnf("Imported %d secret(s)\n", len(pairs))
			}

			if deleteSrc {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("import succeeded but could not remove %s: %w", path, err)
				}
				a.warnf("Removed %s\n", path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&groupName, "group", "", "store all pairs as a single group with this name")
	cmd.Flags().BoolVar(&deleteSrc, "delete", false, "delete the source file after a successful import")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be imported without writing")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite existing entries")
	cmd.Flags().StringVar(&format, "format", "",
		"source format: "+strings.Join(vault.ImportFormats, ", ")+" (default: from the file extension)")
	return cmd
}

// readImportFile parses the source file in the requested format. Every format
// yields the same NAME=value map, so the rest of import is format-agnostic.
func readImportFile(path, format string) (map[string]string, error) {
	switch format {
	case vault.ImportDotenv:
		return readDotenvFile(path)
	case vault.ImportCSV:
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		pairs, err := vault.ParseCSVExport(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return pairs, nil
	case vault.Import1PUX:
		return vault.Parse1PUX(path)
	default:
		return nil, fmt.Errorf("unknown --format %q; use one of %s",
			format, strings.Join(vault.ImportFormats, ", "))
	}
}

// overlap returns the names present in both lists, sorted.
func overlap(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	var out []string
	for _, s := range a {
		if inB[s] {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
