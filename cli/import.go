package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var (
		groupName string
		shred     bool
	)

	cmd := &cobra.Command{
		Use:   "import <file.env>",
		Short: "Import secrets from a .env file",
		Long: "Import KEY=VALUE pairs from a .env file. By default each pair becomes its\n" +
			"own flat secret; use --group to store them all as one grouped entry.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			pairs, err := readDotenvFile(path)
			if err != nil {
				return err
			}
			if len(pairs) == 0 {
				return fmt.Errorf("no KEY=VALUE pairs found in %s", path)
			}

			v, err := vault.New()
			if err != nil {
				return err
			}

			keys := make([]string, 0, len(pairs))
			for k := range pairs {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			if groupName != "" {
				if err := v.AddGroup(groupName, pairs); err != nil {
					return err
				}
				if err := recordGroupMeta(groupName, ""); err != nil {
					return err
				}
				warnf("Imported %d field(s) into group %q\n", len(pairs), groupName)
			} else {
				for _, k := range keys {
					if err := v.AddSecret(k, pairs[k]); err != nil {
						return err
					}
				}
				if err := updateConfig(func(c *vault.Config) {
					if c.Secrets == nil {
						c.Secrets = map[string]vault.SecretMeta{}
					}
					for _, k := range keys {
						m := c.Secrets[k]
						m.Env = k
						c.Secrets[k] = m
						delete(c.Groups, k)
					}
				}); err != nil {
					return err
				}
				warnf("Imported %d secret(s)\n", len(pairs))
			}

			if shred {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("import succeeded but could not remove %s: %w", path, err)
				}
				warnf("Removed %s\n", path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&groupName, "group", "", "store all pairs as a single group with this name")
	cmd.Flags().BoolVar(&shred, "shred", false, "delete the source file after a successful import")
	return cmd
}
