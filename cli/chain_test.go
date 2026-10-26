package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// chain builds the three-level layout a project chain is for: a directory of
// all your code, a directory of one organisation's repositories inside it,
// and one repository inside that. Each level names a group, and the group is
// created outside any repository so only the project file ties it to a
// directory.
//
// It returns the three directories, outermost first, and leaves the test
// standing in the innermost one.
func chain(t *testing.T) (root, mid, leaf string) {
	t.Helper()

	root = t.TempDir()
	mid = filepath.Join(root, "acme")
	leaf = filepath.Join(mid, "bidops")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}

	addGroup(t, "code", "GITHUB_TOKEN=ghp-test-personal\nOPENAI_API_KEY=sk-test-value\n")
	addGroup(t, "acme", "GITHUB_TOKEN=ghp-test-org\n")
	addGroup(t, "bidops", "DATABASE_URL=postgres://test\n")

	writeProject(t, root, "entry = \"code\"\n")
	writeProject(t, mid, "entry = \"acme\"\n")
	writeProject(t, leaf, "entry = \"bidops\"\n")

	t.Chdir(leaf)
	for _, dir := range []string{leaf, mid, root} {
		if _, err := runCmd(t, "", "allow", dir); err != nil {
			t.Fatalf("allow %s: %v", dir, err)
		}
	}
	return root, mid, leaf
}

// addGroup stores a group from a directory that is not a repository, so
// nothing but a project file can claim it.
func addGroup(t *testing.T, name, dotenv string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, dotenv); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", name, "--from-dotenv", src, "--no-project"); err != nil {
		t.Fatalf("add-group %s: %v", name, err)
	}
}

func writeProject(t *testing.T, dir, body string) {
	t.Helper()
	if err := writeFileString(filepath.Join(dir, vault.ProjectFileName), body); err != nil {
		t.Fatal(err)
	}
}

func exportJSON(t *testing.T, args ...string) map[string]string {
	t.Helper()
	out, err := runCmd(t, "", append([]string{"export", "--format", "json"}, args...)...)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("export = %q (%v)", out, err)
	}
	return env
}

// The nearest directory has the last word, and everything above it still
// contributes: the same left-to-right rule as "sanduk exec a b c", applied
// down the tree.
func TestChainResolvesOutermostFirst(t *testing.T) {
	testEnv(t)
	chain(t)

	env := exportJSON(t)
	if env["GITHUB_TOKEN"] != "ghp-test-org" {
		t.Errorf("GITHUB_TOKEN = %q, want the nearer directory's", env["GITHUB_TOKEN"])
	}
	if env["OPENAI_API_KEY"] != "sk-test-value" {
		t.Errorf("OPENAI_API_KEY = %q, want the outermost directory's", env["OPENAI_API_KEY"])
	}
	if env["DATABASE_URL"] != "postgres://test" {
		t.Errorf("DATABASE_URL = %q, want the repository's own", env["DATABASE_URL"])
	}
}

// inherit = false seals a directory: a repository that must resolve the same
// way in CI depends on its own file alone, because the files above it are not
// in the clone.
func TestChainInheritFalseSeals(t *testing.T) {
	testEnv(t)
	_, _, leaf := chain(t)

	writeProject(t, leaf, "entry = \"bidops\"\ninherit = false\n")
	if _, err := runCmd(t, "", "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}

	env := exportJSON(t)
	if _, ok := env["GITHUB_TOKEN"]; ok {
		t.Errorf("a sealed directory must not inherit: %v", env)
	}
	if env["DATABASE_URL"] == "" {
		t.Errorf("a sealed directory keeps its own entry: %v", env)
	}
}

// An unapproved layer fails the command and names the directory to approve.
// Resolving the rest would be a run quietly missing a variable.
func TestChainRefusesUnapprovedAncestor(t *testing.T) {
	testEnv(t)
	root, _, _ := chain(t)

	// Editing the outermost file revokes its approval.
	writeProject(t, root, "entry = \"code\"\n# a later edit\n")

	_, err := runCmd(t, "", "export", "--format", "json")
	if err == nil {
		t.Fatal("expected an unapproved ancestor to fail the command")
	}
	if !strings.Contains(err.Error(), "not approved") || !strings.Contains(err.Error(), "allow") {
		t.Fatalf("error = %v, want it to name the directory to approve", err)
	}
}

// Approving one file says which of the others still need it, instead of
// letting the user find them one failed command at a time.
func TestAllowReportsUnapprovedAbove(t *testing.T) {
	testEnv(t)

	root := t.TempDir()
	leaf := filepath.Join(root, "repo")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	addGroup(t, "code", "GITHUB_TOKEN=ghp-test-personal\n")
	addGroup(t, "repo", "DATABASE_URL=postgres://test\n")
	writeProject(t, root, "entry = \"code\"\n")
	writeProject(t, leaf, "entry = \"repo\"\n")
	t.Chdir(leaf)

	_, status, err := runWithStderr(t, "allow")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !strings.Contains(status, "also applies here and is not approved") {
		t.Fatalf("allow should name the layer above: %q", status)
	}
}

// The listing is the one place the chain is visible, including which layer
// wins a variable two of them provide.
func TestListShowsTheChain(t *testing.T) {
	testEnv(t)
	chain(t)

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"PROJECT", "code", "acme", "bidops", "GITHUB_TOKEN", "shadows code"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ghp-test") || strings.Contains(out, "sk-test") {
		t.Fatalf("list printed a value:\n%s", out)
	}

	var doc struct {
		Project struct {
			Layers []struct {
				Entry   string `json:"entry"`
				Trusted bool   `json:"trusted"`
			} `json:"layers"`
			Keys []struct {
				Name    string   `json:"name"`
				From    string   `json:"from"`
				Shadows []string `json:"shadows"`
			} `json:"keys"`
		} `json:"project"`
	}
	out, err = runCmd(t, "", "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("list --json = %q (%v)", out, err)
	}
	want := []string{"code", "acme", "bidops"}
	if len(doc.Project.Layers) != 3 {
		t.Fatalf("layers = %+v, want %v in resolution order", doc.Project.Layers, want)
	}
	for i, n := range want {
		if doc.Project.Layers[i].Entry != n || !doc.Project.Layers[i].Trusted {
			t.Fatalf("layer %d = %+v, want %q approved", i, doc.Project.Layers[i], n)
		}
	}
	for _, k := range doc.Project.Keys {
		if k.Name != "GITHUB_TOKEN" {
			continue
		}
		if k.From != "acme" || len(k.Shadows) != 1 || k.Shadows[0] != "code" {
			t.Fatalf("GITHUB_TOKEN = %+v, want acme shadowing code", k)
		}
	}
}

// An approved layer still scopes the listing when another is not approved —
// and the unapproved one is shown, so a missing variable has a visible cause
// rather than looking like a vault that lost an entry.
func TestListShowsAnUnapprovedLayer(t *testing.T) {
	testEnv(t)
	root, _, _ := chain(t)
	if _, err := runCmd(t, "sk-test-value", "add", "standalone"); err != nil {
		t.Fatalf("add: %v", err)
	}

	writeProject(t, root, "entry = \"code\"\n# a later edit\n")

	out, status, err := runWithStderr(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "not approved; not applied") {
		t.Errorf("the layer should be shown as not applied:\n%s", out)
	}
	if !strings.Contains(status, "not approved") {
		t.Errorf("stderr should say how to fix it: %q", status)
	}
	// Its keys are not in the block, because they are not injected.
	if strings.Contains(out, "ANTHROPIC_API_KEY") {
		t.Errorf("an unapproved layer must contribute nothing:\n%s", out)
	}
	// The approved layers still narrow the listing.
	if strings.Contains(out, "standalone") {
		t.Errorf("the chain still scopes the view:\n%s", out)
	}
}

// A group tied to a directory under a project chain is the nearer statement,
// so it joins the chain as its innermost layer.
func TestChainPicksUpADeeperLocation(t *testing.T) {
	testEnv(t)
	_, _, leaf := chain(t)

	sub := repoAt(t, filepath.Join(leaf, "worker"))
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "WORKER_TOKEN=test-token\n"); err != nil {
		t.Fatal(err)
	}
	// add-group at a repository root records the location and writes a
	// project file; removing the file leaves the group claimed by the
	// directory in your own config alone, which is the case under test.
	if _, err := runCmd(t, "", "add-group", "worker", "--from-dotenv", src); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	if err := os.Remove(filepath.Join(sub, vault.ProjectFileName)); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "worker") || !strings.Contains(out, "your local record") {
		t.Errorf("the deeper location should be the innermost layer:\n%s", out)
	}
	if !strings.Contains(out, "GITHUB_TOKEN") {
		t.Errorf("the layers above it still apply:\n%s", out)
	}
}
