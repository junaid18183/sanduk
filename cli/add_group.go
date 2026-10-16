package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// parseFieldFlags turns repeated --field KEY flags into field names. A value is
// never accepted on the command line: argv is visible to every process on the
// machine (ps) and lands in shell history, so KEY=VALUE is refused and each
// named field is prompted for instead.
func parseFieldFlags(fields []string) ([]string, error) {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if k, _, ok := strings.Cut(f, "="); ok {
			return nil, fmt.Errorf("--field %s=...: a value on the command line is visible in process listings; "+
				"pass --field %s to be prompted, or use --from-dotenv or --from-json -", k, k)
		}
		if f == "" {
			return nil, fmt.Errorf("--field needs a KEY")
		}
		out = append(out, f)
	}
	return out, nil
}

// readJSONSource reads a JSON object of fields from a file, or from stdin when
// src is "-". Inline JSON is refused for the same reason as --field KEY=VALUE.
func readJSONSource(src string) (map[string]string, error) {
	if strings.HasPrefix(strings.TrimSpace(src), "{") {
		return nil, fmt.Errorf("--from-json takes a file or - (stdin), not inline JSON: argv is visible in process listings")
	}
	var (
		b   []byte
		err error
	)
	if src == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(src)
	}
	if err != nil {
		return nil, err
	}
	return parseJSONObject(string(b))
}

func newAddGroupCmd(a *app) *cobra.Command {
	var (
		fields     []string
		fromJSON   string
		fromDotenv string
		replace    bool
		descFlag   string
	)

	cmd := &cobra.Command{
		Use:   "add-group <name>",
		Short: "Create a grouped secret",
		Long: "Create a group: several named fields stored as one vault entry. Field names\n" +
			"are the environment variable names used by exec and export.\n\n" +
			"Values come from --from-dotenv (a .env file), --from-json (a file, or - for\n" +
			"stdin), or repeated --field KEY, which prompts for KEY's value with echo off.\n" +
			"Values are never taken from argv. When more than one source is given,\n" +
			"--field overrides --from-dotenv, which overrides --from-json.\n\n" +
			"An existing entry is never replaced silently: use \"sanduk group set\" to merge\n" +
			"fields, or pass --replace to overwrite the whole entry.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			m := map[string]string{}
			fieldKeys, err := parseFieldFlags(fields)
			if err != nil {
				return err
			}
			if fromJSON == "-" && len(fieldKeys) > 0 {
				return fmt.Errorf("--from-json - reads stdin, so it cannot be combined with --field prompts")
			}

			if fromJSON != "" {
				parsed, err := readJSONSource(fromJSON)
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
			if len(m) == 0 && len(fieldKeys) == 0 {
				return fmt.Errorf("no fields provided; use --field KEY, --from-dotenv or --from-json")
			}
			// Validate every name before prompting for any value.
			for _, k := range append(sortedMapKeys(m), fieldKeys...) {
				if err := vault.ValidateEnvName(k); err != nil {
					return err
				}
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := a.refuseOverwrite(v, name, replace, "merge with \"sanduk group set "+name+"\", or "); err != nil {
				return err
			}

			for _, k := range fieldKeys {
				val, err := readGroupFieldValue(k, false)
				if err != nil {
					return err
				}
				m[k] = val
			}

			if err := v.AddGroup(name, m); err != nil {
				return err
			}

			if err := a.recordGroupMeta(name, descFlag); err != nil {
				return err
			}

			a.warnf("Added group %q with %d field(s)\n", name, len(m))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&fields, "field", nil, "add field KEY, prompting for its value (repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "read a JSON object of fields from a file, or - for stdin")
	cmd.Flags().StringVar(&fromDotenv, "from-dotenv", "", "read fields from a .env file")
	cmd.Flags().StringVar(&descFlag, "description", "", "human-readable description")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry of any type")
	cmd.MarkFlagsMutuallyExclusive("from-json", "from-dotenv")
	return cmd
}
