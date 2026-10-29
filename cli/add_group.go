package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
				"pass --field %s to be prompted, or use --from-json -", k, k)
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
		fields   []string
		fromJSON string
		replace  bool
		descFlag string
		expires  string
		global   bool
	)

	cmd := &cobra.Command{
		Use:   "add-group <name>",
		Short: "Create a grouped secret",
		Long: "Create a group: several named fields stored as one vault entry. Field names\n" +
			"are the environment variable names used by exec and export.\n\n" +
			"Values come from --from-json (a file, or - for stdin), or repeated --field KEY,\n" +
			"which prompts for KEY's value with echo off. Values are never taken from argv.\n" +
			"To create a group from a .env file, use import --group.\n\n" +
			"An existing entry is never replaced silently: use \"sanduk group set\" to merge\n" +
			"fields, or pass --replace to overwrite the whole entry.\n\n" +
			"With neither --field nor --from-json the group is created empty, and group set\n" +
			"fills it in later.\n\n" +
			"Run at a repository root, or in a folder with no .git above it, this also makes\n" +
			"the group that folder's:\n" +
			"the directory is recorded in your config, so the group is listed there and\n" +
			"left out of listings elsewhere, and a " + vault.ProjectFileName + " naming it is written\n" +
			"and approved unless one already exists. That file is non-secret and meant to\n" +
			"be committed. --global creates a plain global group instead.",
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
			if expires != "" && expires != expiryNone {
				if _, err := vault.ParseExpiry(expires); err != nil {
					return err
				}
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
			if err := a.refuseOverwrite(v, name, replace, "merge with \"sanduk group set -g "+name+" KEY\", or "); err != nil {
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
			if expires != "" {
				if err := a.setExpiry(name, expires); err != nil {
					return err
				}
			}

			a.warnf("Added group %q with %d field(s)\n", name, len(m))

			// A group created inside a repository belongs to it: record that,
			// and give the repository the project file that lets exec and
			// export find the group without being told.
			if !global {
				// A folder with no .git above it is its own project, which is
				// what init does too, so the group is bound to it all the same.
				cwd, err := filepath.Abs(".")
				if err != nil {
					return err
				}
				root, _ := repoRoot(".")
				if root == "" || root == cwd {
					if err := a.adoptProject(name, cwd); err != nil {
						return err
					}
				} else {
					a.warnf("note: %q is a global group; run this from %s, or "+
						"\"sanduk allow\" there, to tie it to that repository\n", name, root)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&fields, "field", nil, "add field KEY, prompting for its value (repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "read a JSON object of fields from a file, or - for stdin")
	cmd.Flags().StringVar(&descFlag, "description", "", "human-readable description")
	cmd.Flags().StringVar(&expires, "expires", "", "rotation deadline as YYYY-MM-DD, or \"none\" to clear it")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry of any type")
	cmd.Flags().BoolVar(&global, "global", false,
		"create a global group: do not tie it to this directory or write "+vault.ProjectFileName)
	return cmd
}
