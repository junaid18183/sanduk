package cli

import (
	"errors"
	"fmt"

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
		fields     []string
		fromJSON   string
		fromDotenv string
		fromFile   string // deprecated alias for --from-dotenv
		replace    bool
		descFlag   string
	)

	cmd := &cobra.Command{
		Use:   "add-group <name>",
		Short: "Create a grouped secret",
		Long: "Create a group: several named fields stored as one vault entry. Field names\n" +
			"are the environment variable names used by exec and export.\n\n" +
			"Values may be supplied with repeated --field K=V, --from-json, or --from-dotenv\n" +
			"(a .env file). When more than one source is given, --field overrides\n" +
			"--from-dotenv, which overrides --from-json.\n\n" +
			"An existing entry is never replaced silently: use \"sanduk group set\" to merge\n" +
			"fields, or pass --replace to overwrite the whole entry.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if fromDotenv == "" && fromFile != "" {
				fromDotenv = fromFile
			}

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
			if fromDotenv != "" {
				parsed, err := readDotenvFile(fromDotenv)
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
			for k := range m {
				if err := vault.ValidateEnvName(k); err != nil {
					return err
				}
			}

			v, err := vault.New()
			if err != nil {
				return err
			}
			if existing, gerr := v.Get(name); gerr == nil {
				if !replace {
					if existing.Type == vault.TypeGroup {
						return fmt.Errorf("group %q already exists with %d field(s); merge with \"sanduk group set %s\", or pass --replace to overwrite it",
							name, len(existing.Fields), name)
					}
					return fmt.Errorf("%q already exists as a %s; pass --replace to overwrite it with a group",
						name, existing.Type)
				}
			} else if !errors.Is(gerr, vault.ErrNotFound) {
				return gerr
			}

			if err := v.AddGroup(name, m); err != nil {
				return err
			}

			if err := recordGroupMeta(name, descFlag); err != nil {
				return err
			}

			warnf("Added group %q with %d field(s)\n", name, len(m))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&fields, "field", nil, "add a field as KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "JSON object of fields")
	cmd.Flags().StringVar(&fromDotenv, "from-dotenv", "", "read fields from a .env file")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "deprecated: use --from-dotenv")
	cmd.Flags().StringVar(&descFlag, "description", "", "human-readable description")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry of any type")
	cmd.MarkFlagsMutuallyExclusive("from-json", "from-dotenv")
	_ = cmd.Flags().MarkDeprecated("from-file", "use --from-dotenv")
	return cmd
}
