package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var (
		groupName string
		deleteSrc bool
		shred     bool // deprecated alias for --delete
		dryRun    bool
		replace   bool
	)

	cmd := &cobra.Command{
		Use:   "import <file.env>",
		Short: "Import secrets from a .env file",
		Long: "Import KEY=VALUE pairs from a .env file. By default each pair becomes its\n" +
			"own flat secret; use --group to store them all as one grouped entry.\n\n" +
			"An existing entry is not overwritten unless --replace is given. With --dry-run\n" +
			"nothing is written and the names that would be imported are printed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !deleteSrc && shred {
				deleteSrc = true
			}

			pairs, err := readDotenvFile(path)
			if err != nil {
				return err
			}
			if len(pairs) == 0 {
				return fmt.Errorf("no KEY=VALUE pairs found in %s", path)
			}

			keys := make([]string, 0, len(pairs))
			for k := range pairs {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			if dryRun {
				for _, k := range keys {
					fmt.Fprintln(cmd.OutOrStdout(), k)
				}
				warnf("dry run: %d key(s) would be imported from %s\n", len(keys), path)
				return nil
			}

			// Validate every key before writing anything, so a bad name cannot
			// leave the import half done.
			for _, k := range keys {
				if err := vault.ValidateEnvName(k); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
			}

			v, err := openVault()
			if err != nil {
				return err
			}

			if groupName != "" {
				existing, gerr := v.Get(groupName)
				switch {
				case gerr == nil && !replace && existing.Type == vault.TypeGroup:
					return fmt.Errorf("group %q already exists with %d field(s); pass --replace to overwrite it",
						groupName, len(existing.Fields))
				case gerr == nil && !replace:
					return fmt.Errorf("%q already exists as a %s; pass --replace to overwrite it with a group",
						groupName, existing.Type)
				case gerr != nil && !errors.Is(gerr, vault.ErrNotFound):
					return gerr
				}
				if err := v.AddGroup(groupName, pairs); err != nil {
					return err
				}
				if err := recordGroupMeta(groupName, ""); err != nil {
					return err
				}
				warnf("Imported %d field(s) into group %q\n", len(pairs), groupName)
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
					warnf("overwriting %d existing entry/entries: %v\n", len(overwrite), overwrite)
				}
				for _, k := range keys {
					if err := v.AddSecret(k, pairs[k]); err != nil {
						return err
					}
				}
				if err := updateConfig(func(c *vault.Config) {
					for _, k := range keys {
						c.RecordSecret(k, k, "")
					}
				}); err != nil {
					return err
				}
				warnf("Imported %d secret(s)\n", len(pairs))
			}

			if deleteSrc {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("import succeeded but could not remove %s: %w", path, err)
				}
				warnf("Removed %s\n", path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&groupName, "group", "", "store all pairs as a single group with this name")
	cmd.Flags().BoolVar(&deleteSrc, "delete", false, "delete the source file after a successful import")
	cmd.Flags().BoolVar(&shred, "shred", false, "deprecated: use --delete")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be imported without writing")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite existing entries")
	_ = cmd.Flags().MarkDeprecated("shred", "use --delete")
	return cmd
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
