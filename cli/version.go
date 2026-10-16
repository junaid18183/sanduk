package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is overridden at build time with:
//
//	-ldflags "-X github.com/junaid18183/sanduk/cli.version=1.2.3"
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sanduk version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "sanduk "+version)
		},
	}
}
