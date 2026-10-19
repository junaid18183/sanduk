package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// maxFileSecret caps what add-file will store. A credential file is a few
// kilobytes; anything far larger is a mistake (a whole keyring, a tarball),
// and some keystores refuse or truncate large items without saying so.
const maxFileSecret = 1 << 20 // 1 MiB

func newAddFileCmd(a *app) *cobra.Command {
	var (
		fromFile    string
		envName     string
		filename    string
		description string
		expires     string
		replace     bool
	)

	cmd := &cobra.Command{
		Use:   "add-file <name>",
		Short: "Store a credential file, exposed to a command as a path",
		Long: "Store the contents of a file that a program insists on reading from a path\n" +
			"rather than an environment variable: a service-account JSON, an App Store\n" +
			"Connect .p8 key, a client certificate.\n\n" +
			"The bytes live in the vault like any other secret. exec hands the child a\n" +
			"path to them without leaving a readable file behind: the data is written to\n" +
			"a temporary file that is unlinked immediately, and the child receives\n" +
			"/dev/fd/N for the descriptor it inherits. Use \"exec --materialize DIR\" for a\n" +
			"tool that needs a real path with an extension.\n\n" +
			"  sanduk add-file gcp --from-file sa.json --as GOOGLE_APPLICATION_CREDENTIALS\n" +
			"  sanduk exec gcp -- gcloud auth activate-service-account --key-file \"$GOOGLE_APPLICATION_CREDENTIALS\"\n\n" +
			"--from-file - reads stdin. --filename sets the basename used when the file\n" +
			"is materialized, for tools that key off the extension.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if fromFile == "" {
				return fmt.Errorf("--from-file is required (use - for stdin)")
			}
			if expires != "" && expires != expiryNone {
				if _, err := vault.ParseExpiry(expires); err != nil {
					return err
				}
			}
			if envName == "" {
				envName = vault.CredentialName(name)
			}
			if err := vault.ValidateEnvName(envName); err != nil {
				return err
			}

			data, err := readFileSecret(fromFile)
			if err != nil {
				return err
			}
			if filename == "" && fromFile != "-" {
				filename = filepath.Base(fromFile)
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			if err := a.refuseOverwrite(v, name, replace, ""); err != nil {
				return err
			}
			if err := v.AddFile(name, data, filename); err != nil {
				return err
			}
			if err := a.updateConfig(func(c *vault.Config) {
				c.RecordFile(name, envName, description)
			}); err != nil {
				return err
			}
			if expires != "" {
				if err := a.setExpiry(name, expires); err != nil {
					return err
				}
			}

			a.warnf("Added file secret %q (%d bytes), exposed as %s\n", name, len(data), envName)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromFile, "from-file", "", "file to store, or - for stdin")
	cmd.Flags().StringVar(&envName, "as", "", "environment variable that will hold the path")
	cmd.Flags().StringVar(&filename, "filename", "",
		"basename to use when the file is materialized (default: the source file's)")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&expires, "expires", "", "rotation deadline as YYYY-MM-DD, or \"none\" to clear it")
	cmd.Flags().BoolVar(&replace, "replace", false, "overwrite an existing entry")
	return cmd
}

// readFileSecret reads the file's bytes, refusing an empty or oversized one.
func readFileSecret(path string) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(io.LimitReader(os.Stdin, maxFileSecret+1))
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", describeSource(path))
	}
	if len(data) > maxFileSecret {
		return nil, fmt.Errorf("%s is larger than %d bytes; a credential file should not be",
			describeSource(path), maxFileSecret)
	}
	return data, nil
}

func describeSource(path string) string {
	if path == "-" {
		return "stdin"
	}
	return path
}
