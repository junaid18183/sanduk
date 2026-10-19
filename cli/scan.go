package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"

	"github.com/junaid18183/sanduk/vault"
	"github.com/spf13/cobra"
)

// maxScanFile skips files too large to be a plausible leak site. A secret
// pasted into a source file, a .env or a log is far below this.
const maxScanFile = 8 << 20 // 8 MiB

// skipDirs are never walked. They hold dependencies and build output rather
// than anything a person pasted a key into, and walking them makes a scan
// slow enough that people stop running it.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "target": true,
	".venv": true, "venv": true, "dist": true, "build": true,
	".next": true, ".tox": true, "__pycache__": true, ".mypy_cache": true,
}

func newScanCmd(a *app) *cobra.Command {
	var (
		scanGit   bool
		scanShell bool
		minLength int
		yes       bool
	)

	cmd := &cobra.Command{
		Use:   "scan [path...]",
		Short: "Search for copies of your stored secrets that have escaped the vault",
		Long: "Look for stored values in places they should never be: the working tree,\n" +
			"git history, your shell history. A match means that secret exists in\n" +
			"plaintext somewhere outside the vault and should be rotated.\n\n" +
			"Findings are reported as an entry name and a location. A value is never\n" +
			"printed, so the output is safe to paste into an issue.\n\n" +
			"Unlike every other command, scan reads every value in the vault — it has to,\n" +
			"to know what to look for. That makes it the one command that decrypts the\n" +
			"whole store, so it asks before doing so, and needs --yes without a terminal.\n" +
			"On a presence-gated backend expect one prompt per entry.\n\n" +
			"  sanduk scan                    # the working tree\n" +
			"  sanduk scan --git              # and everything git has ever stored\n" +
			"  sanduk scan --shell            # and your shell history\n\n" +
			"Secrets are matched in this process and are never passed to another\n" +
			"program as an argument, where every process on the machine could read\n" +
			"them out of the process list. Exits 3 when anything is found.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			roots := args
			if len(roots) == 0 {
				roots = []string{"."}
			}
			if err := a.confirmFullRead(yes); err != nil {
				return err
			}

			v, err := a.openVault()
			if err != nil {
				return err
			}
			values, err := v.ScanValues()
			if err != nil {
				return err
			}
			sc := vault.NewScanner(values, minLength)
			if sc.Needles() == 0 {
				a.warnf("nothing to search for: the vault is empty, or every value is shorter than %d characters\n",
					minLength)
				return nil
			}
			for _, name := range sc.Skipped {
				a.warnf("warning: %s is shorter than %d characters and was not searched for\n", name, minLength)
			}

			var (
				matches  []vault.Match
				searched int
			)
			for _, root := range roots {
				found, n, err := scanTree(sc, root)
				if err != nil {
					return err
				}
				matches = append(matches, found...)
				searched += n
			}
			if scanGit {
				found, err := scanGitHistory(sc, roots[0])
				if err != nil {
					return err
				}
				matches = append(matches, found...)
			}
			if scanShell {
				found, n, err := scanShellHistory(sc)
				if err != nil {
					return err
				}
				matches = append(matches, found...)
				searched += n
			}

			vault.SortMatches(matches)
			return a.reportScan(cmd, matches, searched, sc.Needles())
		},
	}

	cmd.Flags().BoolVar(&scanGit, "git", false, "also search the repository's history (git log -p --all)")
	cmd.Flags().BoolVar(&scanShell, "shell", false, "also search your shell history files")
	cmd.Flags().IntVar(&minLength, "min-length", vault.DefaultScanMinLength,
		"ignore stored values shorter than this, which would match everywhere")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation that scan reads every value")
	return cmd
}

// confirmFullRead gets explicit agreement before decrypting the whole vault.
func (a *app) confirmFullRead(yes bool) error {
	if yes {
		return nil
	}
	if !isTerminal(os.Stdin) {
		return fmt.Errorf("scan reads every value in the vault; pass --yes to confirm (stdin is not a terminal)")
	}
	answer, err := promptLine("scan reads every value in the vault. Continue? [y/N] ")
	if err != nil {
		return err
	}
	if answer != "y" && answer != "Y" && answer != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}

// reportScan prints the findings and chooses the exit status.
func (a *app) reportScan(cmd *cobra.Command, matches []vault.Match, searched, needles int) error {
	if len(matches) == 0 {
		a.warnf("no leaks found: %d value(s) searched for across %d file(s)\n", needles, searched)
		return nil
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "ENTRY\tLOCATION")
	for _, m := range matches {
		where := m.Where
		if m.Line > 0 {
			where = fmt.Sprintf("%s:%d", where, m.Line)
		}
		if m.Note != "" {
			where += "  (" + m.Note + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\n", m.Entry, where)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return &vault.ExitError{Code: exitDrift, Err: fmt.Errorf(
		"%d leaked copy/copies found; rotate the affected secrets", len(matches))}
}

// scanTree walks root and searches every regular text file.
//
// Files are opened through an os.Root anchored at the scan root, so a symlink
// inside the tree cannot make the scanner read something outside it — and
// cannot be swapped for one between the walk and the open.
func scanTree(sc *vault.Scanner, root string) ([]vault.Match, int, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, 0, fmt.Errorf("scanning %s: %w", root, err)
	}
	if !info.IsDir() {
		// Scanning a single named file is a reasonable thing to ask for.
		return scanOneFile(sc, filepath.Dir(root), filepath.Base(root))
	}

	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, 0, fmt.Errorf("scanning %s: %w", root, err)
	}
	defer dir.Close()

	var (
		matches  []vault.Match
		searched int
	)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory is not a reason to abandon the scan.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		// Type() comes from the directory entry, so a symlink is skipped here
		// rather than followed; its target is scanned on its own if it lives
		// inside the tree.
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() == 0 || info.Size() > maxScanFile {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		f, err := dir.Open(rel)
		if err != nil {
			return nil
		}
		defer f.Close()
		found, ok, err := sc.ScanReader(path, f)
		if err != nil {
			return err
		}
		if ok {
			searched++
		}
		matches = append(matches, found...)
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scanning %s: %w", root, err)
	}
	return matches, searched, nil
}

// scanOneFile searches a single named file, opened through its directory so
// the same root-scoped rules apply.
func scanOneFile(sc *vault.Scanner, dir, name string) ([]vault.Match, int, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	matches, ok, err := sc.ScanReader(filepath.Join(dir, name), f)
	if err != nil {
		return nil, 0, err
	}
	searched := 0
	if ok {
		searched = 1
	}
	return matches, searched, nil
}

// scanGitHistory searches every blob git has ever recorded. The secrets are
// matched against the command's output here rather than handed to `git grep`,
// which would put them in argv for every process on the machine to read.
func scanGitHistory(sc *vault.Scanner, dir string) ([]vault.Match, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("--git needs git on PATH")
	}
	cmd := exec.Command("git", "-C", dir, "log", "--all", "-p", "--no-color")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("running git log: %w", err)
	}
	matches, _, scanErr := sc.ScanReader("git history", out)
	// Drain anything left, or git blocks on a full pipe and never exits.
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git log failed (is %s a repository?)", dir)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	// Line numbers are meaningless in a diff stream.
	for i := range matches {
		matches[i].Line = 0
		matches[i].Note = "run: git log -S <value> to find the commit"
	}
	return matches, nil
}

// scanShellHistory searches the usual interactive-shell history files.
func scanShellHistory(sc *vault.Scanner) ([]vault.Match, int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, 0, err
	}
	candidates := []string{
		filepath.Join(home, ".bash_history"),
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".sh_history"),
		filepath.Join(home, ".local", "share", "fish", "fish_history"),
	}
	if h := os.Getenv("HISTFILE"); h != "" {
		candidates = append(candidates, h)
	}

	var (
		matches  []vault.Match
		searched int
	)
	for _, path := range candidates {
		// The history paths are this user's own, including $HISTFILE.
		f, err := os.Open(path) //nolint:gosec // the user's own shell history, by definition
		if err != nil {
			continue
		}
		found, ok, err := sc.ScanReader(path, f)
		f.Close()
		if err != nil {
			return nil, 0, err
		}
		if ok {
			searched++
		}
		matches = append(matches, found...)
	}
	return matches, searched, nil
}
