package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// completeEntryNames suggests vault entry names for a command's first argument.
func completeEntryNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return entryNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeSelector suggests entry names for exec/export selectors, which start
// as a bare entry name.
func completeSelector(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return entryNames(), cobra.ShellCompDirectiveNoFileComp
}

func entryNames() []string {
	if !completionCanReadVault() {
		return nil
	}
	v, err := openVault()
	if err != nil {
		return nil
	}
	names, err := v.List()
	if err != nil {
		return nil
	}
	return names
}

// completionCanReadVault reports whether listing entries can be done without an
// interactive passphrase prompt, which would corrupt shell completion. The
// encrypted file backend prompts unless its passphrase is already available in
// the environment.
func completionCanReadVault() bool {
	switch currentSettings().Backend {
	case "file", "age":
		return os.Getenv("SANDUK_FILE_PASSPHRASE") != ""
	}
	return true
}
