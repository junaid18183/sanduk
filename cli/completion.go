package cli

import (
	"errors"
	"io"
	"os"

	"github.com/junaid18183/sanduk/vault"

	"github.com/spf13/cobra"
)

// a.completeEntryNames suggests vault entry names for a command's first argument.
func (a *app) completeEntryNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return a.entryNames(), cobra.ShellCompDirectiveNoFileComp
}

// a.completeSelector suggests entry names for exec/export selectors, which start
// as a bare entry name.
func (a *app) completeSelector(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return a.entryNames(), cobra.ShellCompDirectiveNoFileComp
}

// entryNames lists entry names for completion. It must never prompt — a prompt
// would corrupt the shell's completion output — so the age passphrase comes
// only from SANDUK_FILE_PASSPHRASE; without it, completion offers nothing.
func (a *app) entryNames() []string {
	s := a.resolved()
	if a.settingsErr != nil {
		return nil
	}
	o := a.options(s)
	o.PassFunc = func(string) (string, error) {
		if p := os.Getenv("SANDUK_FILE_PASSPHRASE"); p != "" {
			return p, nil
		}
		return "", errors.New("no passphrase available without prompting")
	}
	o.Warn = io.Discard
	v, err := vault.Open(o)
	if err != nil {
		return nil
	}
	names, err := v.List()
	if err != nil {
		return nil
	}
	return names
}
