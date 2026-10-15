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
			"information (entry names, environment variable mappings, descriptions).",
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

			fmt.Fprintf(os.Stderr, "# %s\n", path)
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
