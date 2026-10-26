package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newInitCmd(a *app) *cobra.Command {
	var (
		include      []string
		only         []string
		fromDotenv   string
		fromTemplate string
		envrc        bool
		force        bool
	)

	cmd := &cobra.Command{
		Use:   "init [entry]",
		Short: "Set this repository up: write and approve its " + vault.ProjectFileName,
		Long: "Declare which vault entry this repository uses, before it has one.\n\n" +
			"It writes " + vault.ProjectFileName + " naming the entry (the directory's name by\n" +
			"default), approves it, and records the directory against the group so\n" +
			"\"sanduk list\" shows it here and leaves it out elsewhere. The entry does not\n" +
			"have to exist yet — create it afterwards with \"sanduk group set\", or pass\n" +
			"--from-dotenv to seed it now.\n\n" +
			"  sanduk init                             # entry named after the directory\n" +
			"  sanduk init --from-template             # fill in this repo's .env.example\n" +
			"  sanduk init contexthub --from-dotenv .env\n" +
			"  sanduk init --include openai,agents     # global entries merged underneath\n\n" +
			"--from-template is the usual way into somebody else's project: it reads the\n" +
			"key names from a committed " + defaultTemplateName + " (or .env.template) and prompts for\n" +
			"each value with echo off. The placeholder values in the file are never\n" +
			"stored, and an empty answer skips a key, so \"sanduk doctor\" reports it as\n" +
			"missing until you come back to it. The keys you fill in become the `only`\n" +
			"list, which sanduk then keeps in step with the group.\n\n" +
			"--envrc also writes a direnv .envrc that exports the entry on cd. It is\n" +
			"off by default: direnv exports into the shell, so every process started\n" +
			"from it inherits the secrets, including an editor or agent opened later.\n" +
			"\"sanduk exec -- <cmd>\" keeps them in one child instead.\n\n" +
			"--include names the global secrets and groups this repository also needs.\n" +
			"They resolve before the project's own entry, so the project wins on a\n" +
			"variable both provide. --only records the keys the project expects; once\n" +
			"recorded, sanduk keeps that list in step as the group gains and loses\n" +
			"fields. The file is non-secret and meant to be committed.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			name := filepath.Base(dir)
			if len(args) == 1 {
				name = args[0]
			}
			if err := vault.ValidateEntryName(name); err != nil {
				return err
			}
			for _, inc := range include {
				if err := vault.ValidateEntryName(inc); err != nil {
					return err
				}
				if inc == name {
					return fmt.Errorf("--include %s is the entry itself", inc)
				}
			}
			for _, k := range only {
				if err := vault.ValidateEnvName(k); err != nil {
					return err
				}
			}

			path := filepath.Join(dir, vault.ProjectFileName)
			switch _, err := os.Stat(path); {
			case err == nil && !force:
				return fmt.Errorf("%s already exists; edit it, or pass --force to replace it", path)
			case err != nil && !os.IsNotExist(err):
				return err
			}

			if fromTemplate != "" {
				keys, path, err := templateKeys(dir, fromTemplate)
				if err != nil {
					return err
				}
				if !isTerminal(os.Stdin) {
					return fmt.Errorf("--from-template prompts for each value, which needs a " +
						"terminal; use --from-dotenv for a non-interactive run")
				}
				a.warnf("%s lists %d key(s); enter each value, or press enter to skip it.\n",
					path, len(keys))
				fields := map[string]string{}
				for _, k := range keys {
					val, ok, err := readTemplateValue(k)
					if err != nil {
						return err
					}
					if ok {
						fields[k] = val
					}
				}
				if len(fields) == 0 {
					return fmt.Errorf("no values entered; nothing to store")
				}
				v, err := a.openVault()
				if err != nil {
					return err
				}
				if err := v.MergeGroup(name, fields); err != nil {
					return err
				}
				if err := a.recordGroupMeta(name, ""); err != nil {
					return err
				}
				a.warnf("Stored %d of %d key(s) in group %q\n", len(fields), len(keys), name)
				if len(only) == 0 {
					// The template is the project's key list, so it is also
					// the only list — of what was actually stored, since a
					// key the group lacks would fail --only.
					only = sortedMapKeys(fields)
				}
				if n := len(keys) - len(fields); n > 0 {
					a.warnf("%d key(s) skipped; \"sanduk doctor\" will report them as missing\n", n)
				}
			}

			// Seed the group first: a project file naming an entry that the
			// next command creates is fine, but failing after writing the
			// file would leave the repository half set up.
			if fromDotenv != "" {
				fields, err := readDotenvFile(fromDotenv)
				if err != nil {
					return err
				}
				if len(fields) == 0 {
					return fmt.Errorf("%s has no fields", fromDotenv)
				}
				for _, k := range sortedMapKeys(fields) {
					if err := vault.ValidateEnvName(k); err != nil {
						return err
					}
				}
				v, err := a.openVault()
				if err != nil {
					return err
				}
				if err := v.MergeGroup(name, fields); err != nil {
					return err
				}
				if err := a.recordGroupMeta(name, ""); err != nil {
					return err
				}
				a.warnf("Stored %d field(s) in group %q\n", len(fields), name)
			}

			if err := vault.WriteFileAtomic(path, initFileBytes(name, include, only), 0o644); err != nil {
				return err
			}
			_, proj, err := vault.LoadProject(dir)
			if err != nil {
				return err
			}
			if proj == nil {
				return fmt.Errorf("wrote %s but could not read it back", path)
			}
			// Approving what sanduk just wrote: these bytes came from this
			// command, not from a repository. A later edit revokes it as
			// usual.
			if _, err := vault.ApproveProject(a.trustPath(), proj); err != nil {
				// The file is written but not approved. Say both, so the
				// next step is "fix this, then sanduk allow" rather than a
				// re-run of init that now refuses the file it just wrote.
				return fmt.Errorf("wrote %s but could not approve it: %w\n"+
					"fix that, then run \"sanduk allow\" here", path, err)
			}
			a.warnf("Wrote and approved %s — commit it: it names the entry, never a value\n", path)

			if envrc {
				if err := a.writeEnvrc(dir, force); err != nil {
					return err
				}
			}

			var recorded bool
			if err := a.updateConfig(func(c *vault.Config) {
				recorded = c.SetGroupLocation(name, dir)
			}); err != nil {
				return err
			}
			if recorded {
				a.warnf("Recorded %q as the group for %s\n", name, dir)
			} else {
				a.warnf("Next: sanduk group set %s KEY   (the entry does not exist yet)\n", name)
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&include, "include", nil,
		"global entries merged underneath this one (repeatable, or comma-separated)")
	cmd.Flags().StringSliceVar(&only, "only", nil,
		"the keys this project expects; kept in step with the group afterwards")
	cmd.Flags().StringVar(&fromDotenv, "from-dotenv", "", "seed the entry from a .env file")
	cmd.Flags().StringVar(&fromTemplate, "from-template", "",
		"prompt for each key named by a committed template (default: this repo's "+
			defaultTemplateName+" or .env.template)")
	// --from-template with no value means "the one this repository has".
	cmd.Flags().Lookup("from-template").NoOptDefVal = "auto"
	cmd.MarkFlagsMutuallyExclusive("from-dotenv", "from-template")
	cmd.Flags().BoolVar(&envrc, "envrc", false,
		"also write a direnv .envrc that exports the entry on cd (see the warning in --help)")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing "+vault.ProjectFileName+" or .envrc")
	return cmd
}

// initFileBytes renders the project file, explaining itself to the next person
// who opens it in the repository.
func initFileBytes(entry string, include, only []string) []byte {
	var b strings.Builder
	b.WriteString("# Non-secret — safe to commit. Names the vault entry holding this repo's\n")
	b.WriteString("# secrets, so `sanduk exec/export` can be run without naming it.\n")
	b.WriteString("# Approve once per machine (and after any edit here): sanduk allow\n")
	if len(include) > 0 {
		b.WriteString("\n# Global entries merged underneath, in order; the entry below wins.\n")
		b.WriteString("include = [" + quoteInOrder(include) + "]\n")
	}
	b.WriteString("entry = " + strconv.Quote(entry) + "\n")
	if len(only) > 0 {
		b.WriteString("\n# The keys this project expects. sanduk keeps this in step as the\n")
		b.WriteString("# group gains and loses fields; delete the line to opt out.\n")
		b.WriteString(vault.FormatOnly(only) + "\n")
	}
	return []byte(b.String())
}

// quoteInOrder renders names for the file as written. Unlike quoteList, which
// tidies an error message, this must not sort: include is a merge order.
func quoteInOrder(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = strconv.Quote(s)
	}
	return strings.Join(quoted, ", ")
}

// templateKeys reads the key names a committed template lists. The values in
// such a file are placeholders — "changeme", "sk-...", or empty — so only the
// names are taken, and nothing from the file is ever stored.
func templateKeys(dir, flag string) ([]string, string, error) {
	path := flag
	if flag == "auto" {
		if path = findTemplate(dir); path == "" {
			return nil, "", fmt.Errorf("no template here (looked for %s); pass --from-template FILE",
				strings.Join(templateNames, ", "))
		}
	}
	fields, err := readDotenvFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(fields) == 0 {
		return nil, "", fmt.Errorf("%s lists no keys", path)
	}
	keys := sortedMapKeys(fields)
	for _, k := range keys {
		if err := vault.ValidateEnvName(k); err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
	}
	return keys, path, nil
}

// readTemplateValue prompts for one key, confirming it, and reports whether a
// value was given. An empty answer skips the key rather than failing: filling
// in a long template in one sitting is not always possible, and doctor
// reports what is still missing.
func readTemplateValue(key string) (string, bool, error) {
	val, err := promptSecret(key + ": ")
	if err != nil || val == "" {
		return "", false, err
	}
	again, err := promptSecret(key + " (confirm): ")
	if err != nil {
		return "", false, err
	}
	if again != val {
		return "", false, fmt.Errorf("values do not match for %s", key)
	}
	return val, true, nil
}

// envrcName is direnv's per-directory script.
const envrcName = ".envrc"

// writeEnvrc writes a direnv script that exports the project's entry, with the
// warning that belongs next to it: direnv puts the values in the shell, not in
// one child, so everything started from that shell inherits them.
func (a *app) writeEnvrc(dir string, force bool) error {
	path := filepath.Join(dir, envrcName)
	switch _, err := os.Stat(path); {
	case err == nil && !force:
		return fmt.Errorf("%s already exists; edit it, or pass --force to replace it", path)
	case err != nil && !os.IsNotExist(err):
		return err
	}
	if err := vault.WriteFileAtomic(path, envrcBytes(), 0o644); err != nil {
		return err
	}
	a.warnf("Wrote %s — run \"direnv allow\" to enable it\n", path)
	a.warnf("note: direnv exports into your shell, so every process started from it " +
		"inherits these values; prefer \"sanduk exec -- <cmd>\" where you can\n")
	return nil
}

func envrcBytes() []byte {
	return []byte("# direnv entry point. Nothing here is a secret: it names no value and\n" +
		"# reads them from the keystore at load time.\n" +
		"#\n" +
		"# These land in your *shell*, so every process you start from it inherits\n" +
		"# them — an editor, a test runner, an agent. `sanduk exec -- <cmd>` puts\n" +
		"# them in that one child instead; prefer it where you can.\n" +
		"#\n" +
		"# First time on a machine: sanduk allow\n" +
		"if has sanduk; then\n" +
		"  eval \"$(sanduk export --format shell)\" || \\\n" +
		"    log_error \"sanduk export failed — run 'sanduk allow' in this repo\"\n" +
		"else\n" +
		"  log_error \"sanduk is not installed — secrets will be missing\"\n" +
		"fi\n")
}
