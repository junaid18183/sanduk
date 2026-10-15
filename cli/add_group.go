package cli

import (
	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// parseFieldFlags turns repeated KEY=VALUE flags into a map.
func parseFieldFlags(fields []string) (map[string]string, error) {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		k, v, ok := cut(f, '=')
		if !ok || k == "" {
			return nil, errInvalidField(f)
		}
		out[k] = v
	}
	return out, nil
}

func newAddGroupCmd() *cobra.Command {
	var (
		fields      []string
		fromJSON    string
		fromFile    string
		description string
	)

	cmd := &cobra.Command{
		Use:   "add-group <name>",
		Short: "Add or update a grouped secret",
		Long: "Add or update a group: several named fields stored as one vault entry.\n" +
			"Field names are the environment variable names used by exec and export.\n" +
			"Values may be supplied with repeated --field K=V, --from-json, or\n" +
			"--from-file (a .env file).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			m := map[string]string{}
			if fromJSON != "" {
				parsed, err := parseJSONObject(fromJSON)
				if err != nil {
					return err
				}
				for k, v := range parsed {
					m[k] = v
				}
			}
			if fromFile != "" {
				parsed, err := readDotenvFile(fromFile)
				if err != nil {
					return err
				}
				for k, v := range parsed {
					m[k] = v
				}
			}
			fromFlags, err := parseFieldFlags(fields)
			if err != nil {
				return err
			}
			for k, v := range fromFlags {
				m[k] = v
			}

			if len(m) == 0 {
				return errNoFields()
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			if err := v.AddGroup(name, m); err != nil {
				return err
			}

			if err := recordGroupMeta(name, description); err != nil {
				return err
			}

			warnf("Added group %q with %d field(s)\n", name, len(m))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&fields, "field", nil, "add a field as KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "JSON object of fields")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "read fields from a .env file")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	return cmd
}
