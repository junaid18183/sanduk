package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newConfigCmd(a *app) *cobra.Command {
	var (
		pathOnly bool
		edit     bool
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the location and contents of the metadata config",
		Long: "Show the sanduk metadata config. This file holds only non-secret\n" +
			"information: backend settings (which are defaults, overridable by a flag or\n" +
			"an environment variable), entry names, environment variable mappings, and\n" +
			"descriptions.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := a.configPath()

			if edit {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				c := exec.Command(editor, path) //nolint:gosec // $EDITOR is the user's own choice of program
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				return c.Run()
			}

			if pathOnly {
				fmt.Fprintln(cmd.OutOrStdout(), path)
				return nil
			}

			// The header goes to stderr, so stdout stays exactly the file and
			// `sanduk config > copy.toml` remains useful.
			a.warnf("# %-8s %s\n", "path:", path)
			s := a.resolved()
			if a.settingsErr != nil {
				a.warnf("# %-8s unreadable: %v\n", "config:", a.settingsErr)
			}
			a.warnf("# %-8s %s (from %s)\n", "backend:", orDefault(s.Backend, "platform default"), s.BackendFrom)
			a.warnf("# %-8s %s (from %s)\n", "keychain:", s.Keychain, s.KeychainFrom)
			a.warnf("# %-8s %s (from %s)\n", "service:", s.Service, s.ServiceFrom)
			a.warnf("# %s\n", "flags beat environment variables, which beat this file")

			b, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					a.warnf("# (not created yet)\n")
					return nil
				}
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}

	cmd.Flags().BoolVar(&pathOnly, "path", false, "print only the config file path")
	cmd.Flags().BoolVar(&edit, "edit", false, "open the config in $EDITOR")
	return cmd
}

// orDefault renders an unset value readably.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
