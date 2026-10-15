package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
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
			path := cfgFile
			if path == "" {
				path = vault.ConfigPath()
			}

			if edit {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				c := exec.Command(editor, path)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				return c.Run()
			}

			if pathOnly {
				fmt.Println(path)
				return nil
			}

			// The header goes to stderr, so stdout stays exactly the file and
			// `sanduk config > copy.toml` remains useful.
			warnf("# %-8s %s\n", "path:", path)
			switch {
			case settingsErr != nil:
				warnf("# %-8s unreadable: %v\n", "settings:", settingsErr)
			case resolved != nil:
				warnf("# %-8s %s (from %s)\n", "backend:", orDefault(resolved.Backend, "platform default"), resolved.BackendFrom)
				warnf("# %-8s %s (from %s)\n", "keychain:", resolved.Keychain, resolved.KeychainFrom)
				warnf("# %-8s %s (from %s)\n", "service:", resolved.Service, resolved.ServiceFrom)
				warnf("# %s\n", "flags beat environment variables, which beat this file")
			}

			b, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					warnf("# (not created yet)\n")
					return nil
				}
				return err
			}
			_, err = os.Stdout.Write(b)
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
