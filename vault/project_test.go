package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectWalksUp(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ProjectFileName), "entry = \"warden\"\nonly = [\"DB_URL\"]\n")

	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	path, proj, err := LoadProject(deep)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if proj == nil {
		t.Fatal("expected a project file to be found")
	}
	if proj.Entry != "warden" {
		t.Fatalf("entry = %q, want warden", proj.Entry)
	}
	if len(proj.Only) != 1 || proj.Only[0] != "DB_URL" {
		t.Fatalf("only = %v, want [DB_URL]", proj.Only)
	}
	if want := filepath.Join(root, ProjectFileName); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestLoadProjectNearestWins(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ProjectFileName), "entry = \"outer\"\n")

	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(inner, ProjectFileName), "entry = \"inner\"\n")

	_, proj, err := LoadProject(inner)
	if err != nil {
		t.Fatal(err)
	}
	if proj.Entry != "inner" {
		t.Fatalf("entry = %q, want the nearest file to win", proj.Entry)
	}
}

func TestLoadProjectAbsent(t *testing.T) {
	path, proj, err := LoadProject(t.TempDir())
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if path != "" || proj != nil {
		t.Fatalf("expected no project, got (%q, %+v)", path, proj)
	}
}

func TestLoadProjectRequiresEntry(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ProjectFileName), "only = [\"A\"]\n")

	if _, _, err := LoadProject(dir); err == nil {
		t.Fatal("expected an error for a project file without an entry")
	}
}

func TestReplaceOnlyPreservesTheRestOfTheFile(t *testing.T) {
	src := []byte("# why this file exists\n" +
		"include = [\"openai\"]\n" +
		"entry = \"warden\"\n" +
		"only = [\"DB_URL\"]\n" +
		"# trailing note\n")

	out, ok := ReplaceOnly(src, []string{"DB_URL", "API_KEY"})
	if !ok {
		t.Fatal("ReplaceOnly did not find the assignment")
	}
	got := string(out)
	if !strings.Contains(got, `only = ["DB_URL", "API_KEY"]`) {
		t.Fatalf("list not rewritten:\n%s", got)
	}
	// Everything the user wrote around it survives: this is a committed file,
	// and re-encoding it would drop the comments that explain it.
	for _, want := range []string{"# why this file exists", `include = ["openai"]`,
		`entry = "warden"`, "# trailing note"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewrite lost %q:\n%s", want, got)
		}
	}
	// And it still parses to the new list.
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ProjectFileName), got)
	_, p, err := LoadProject(dir)
	if err != nil || p == nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(p.Only) != 2 || p.Only[0] != "DB_URL" || p.Only[1] != "API_KEY" {
		t.Fatalf("only = %v", p.Only)
	}
}

func TestReplaceOnlyHandlesMultilineAndAbsentLists(t *testing.T) {
	multi := []byte("entry = \"warden\"\nonly = [\n  \"DB_URL\",\n  \"API_KEY\",\n]\nkeep = 1\n")
	out, ok := ReplaceOnly(multi, []string{"DB_URL"})
	if !ok {
		t.Fatal("a multi-line list should be found")
	}
	if got := string(out); !strings.Contains(got, `only = ["DB_URL"]`) || !strings.Contains(got, "keep = 1") {
		t.Fatalf("multi-line rewrite = %q", got)
	}

	// No list: sanduk keeps one current, it does not start one.
	none := []byte("entry = \"warden\"\n")
	if out, ok := ReplaceOnly(none, []string{"DB_URL"}); ok || string(out) != string(none) {
		t.Fatalf("ReplaceOnly invented a list: %q (ok=%v)", out, ok)
	}
	// A commented-out list is not an assignment either.
	commented := []byte("entry = \"warden\"\n# only = [\"DB_URL\"]\n")
	if _, ok := ReplaceOnly(commented, []string{"X"}); ok {
		t.Fatal("a commented line must not be treated as the assignment")
	}
	// Neither is a key that merely ends in "only".
	other := []byte("entry = \"warden\"\nreadonly = true\n")
	if _, ok := ReplaceOnly(other, []string{"X"}); ok {
		t.Fatal("readonly is not only")
	}
}

// A chain is every project file above a directory, outermost first: the order
// they resolve in, so the nearest one wins.
func TestLoadProjectChain(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "acme")
	leaf := filepath.Join(mid, "bidops")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, body := range map[string]string{
		root: "entry = \"code\"\n",
		mid:  "entry = \"acme\"\n",
		leaf: "entry = \"bidops\"\ninclude = [\"openai\"]\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, ProjectFileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	chain, err := LoadProjectChain(leaf)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	want := []string{"code", "acme", "bidops"}
	if len(chain) != len(want) {
		t.Fatalf("chain = %d files, want %v", len(chain), want)
	}
	for i, n := range want {
		if chain[i].Entry != n {
			t.Fatalf("chain = %v, want %v outermost first", entryNames(chain), want)
		}
		if !chain[i].Inherits() {
			t.Fatalf("%s should inherit by default", n)
		}
	}
	if chain[2].Dir() != leaf {
		t.Fatalf("Dir() = %q, want %q", chain[2].Dir(), leaf)
	}

	// The nearest file alone is still what LoadProject returns.
	_, proj, err := LoadProject(leaf)
	if err != nil || proj == nil || proj.Entry != "bidops" {
		t.Fatalf("LoadProject = %+v, %v", proj, err)
	}
}

// inherit = false stops the walk at the file that sets it — the file itself
// still applies, nothing above it does.
func TestLoadProjectChainSeals(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "acme")
	leaf := filepath.Join(mid, "bidops")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, body := range map[string]string{
		root: "entry = \"code\"\n",
		mid:  "entry = \"acme\"\ninherit = false\n",
		leaf: "entry = \"bidops\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, ProjectFileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	chain, err := LoadProjectChain(leaf)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if got := entryNames(chain); len(got) != 2 || got[0] != "acme" || got[1] != "bidops" {
		t.Fatalf("chain = %v, want [acme bidops]: the seal keeps its own file", got)
	}
}

// No project file anywhere above is a normal state, not an error.
func TestLoadProjectChainEmpty(t *testing.T) {
	chain, err := LoadProjectChain(t.TempDir())
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain) != 0 {
		t.Fatalf("chain = %v, want none", entryNames(chain))
	}
}

// A file that exists but does not parse must stop the walk: skipping it would
// resolve something other than what the directory asked for.
func TestLoadProjectChainRefusesABrokenLayer(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "repo")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ProjectFileName), []byte("entry = \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leaf, ProjectFileName), []byte("entry = \"repo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectChain(leaf); err == nil {
		t.Fatal("expected an unparseable layer to be an error")
	}
}

func entryNames(chain []*Project) []string {
	out := make([]string, 0, len(chain))
	for _, p := range chain {
		out = append(out, p.Entry)
	}
	return out
}
