package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

// runWithStderr runs sanduk and also returns its status messages, which is
// where a listing explains what it left out.
func runWithStderr(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd(newApp())
	var out, status bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&status)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), status.String(), err
}

// repoAt makes dir look like a repository root and moves the test into it.
func repoAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// addProjectGroup creates a group from inside a repository, which is the flow
// that ties it to that directory.
func addProjectGroup(t *testing.T, name string) string {
	t.Helper()
	dir := repoAt(t, t.TempDir())
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "DB_URL=postgres://test\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", name, "--from-dotenv", src, "--description", "project group"); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	return dir
}

func TestAddGroupAdoptsItsRepository(t *testing.T) {
	cfgPath := testEnv(t)
	dir := addProjectGroup(t, "warden")

	// The project file is written for the repository to commit...
	b, err := os.ReadFile(filepath.Join(dir, vault.ProjectFileName))
	if err != nil {
		t.Fatalf("no project file written: %v", err)
	}
	if !strings.Contains(string(b), `entry = "warden"`) {
		t.Fatalf("project file = %q", b)
	}
	// ...and approved, because sanduk wrote those bytes itself.
	_, proj, err := vault.LoadProject(dir)
	if err != nil || proj == nil {
		t.Fatalf("LoadProject: %v", err)
	}
	trusted, err := vault.IsProjectTrusted(filepath.Join(filepath.Dir(cfgPath), vault.TrustFileName), proj)
	if err != nil || !trusted {
		t.Fatalf("project file should be approved (trusted=%v err=%v)", trusted, err)
	}

	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["warden"].Location; got != dir {
		t.Fatalf("location = %q, want %q", got, dir)
	}

}

// An existing project file is never rewritten: it may name another entry on
// purpose, and it is the repository's file, not sanduk's.
func TestAddGroupKeepsAnExistingProjectFile(t *testing.T) {
	cfgPath := testEnv(t)
	dir := repoAt(t, t.TempDir())
	path := filepath.Join(dir, vault.ProjectFileName)
	if err := writeFileString(path, "entry = \"kept\"\n"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "K=v\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "second", "--from-dotenv", src); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), `entry = "kept"`) {
		t.Fatalf("project file = %q (%v), want it untouched", b, err)
	}
	// The directory is still recorded, so the new group is listed here.
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["second"].Location; got != dir {
		t.Fatalf("location = %q, want %q", got, dir)
	}
}

func TestAddGroupOutsideARepositoryStaysGlobal(t *testing.T) {
	cfgPath := testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "K=v\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "loose", "--from-dotenv", src); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, vault.ProjectFileName)); !os.IsNotExist(err) {
		t.Fatal("add-group wrote a project file outside a repository")
	}
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["loose"].Location; got != "" {
		t.Fatalf("location = %q, want none", got)
	}
}

func TestNoProjectKeepsAddGroupGlobal(t *testing.T) {
	cfgPath := testEnv(t)
	dir := repoAt(t, t.TempDir())
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "K=v\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "loose", "--from-dotenv", src, "--no-project"); err != nil {
		t.Fatalf("add-group --no-project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, vault.ProjectFileName)); !os.IsNotExist(err) {
		t.Fatal("--no-project still wrote a project file")
	}
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["loose"].Location; got != "" {
		t.Fatalf("--no-project recorded location %q", got)
	}
}

// The whole point of the scope: a repository shows its own entry and the
// entries that stand on their own, and nothing else.
func TestListScopes(t *testing.T) {
	testEnv(t)
	repo := addProjectGroup(t, "warden")

	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add openai: %v", err)
	}
	if _, err := runCmd(t, "sk-test-value", "add", "standalone"); err != nil {
		t.Fatalf("add standalone: %v", err)
	}
	// Linking makes openai part of warden rather than a top-level entry.
	if _, err := runCmd(t, "", "group", "link", "warden", "openai"); err != nil {
		t.Fatalf("group link: %v", err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"PROJECT", "warden", "DB_URL", "OPENAI_API_KEY", "-> openai", "standalone"} {
		if !strings.Contains(out, want) {
			t.Errorf("list inside the repo lacks %q:\n%s", want, out)
		}
	}
	// openai is shown as warden's source, not as an entry of its own.
	if strings.Contains(out, "\nopenai ") {
		t.Errorf("a linked secret should not be listed on its own:\n%s", out)
	}
	if strings.Contains(out, "sk-test") || strings.Contains(out, "postgres") {
		t.Fatalf("list printed a value:\n%s", out)
	}

	// Outside the repository the project's group is gone from the listing.
	t.Chdir(t.TempDir())
	out, err = runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list outside: %v", err)
	}
	if strings.Contains(out, "warden") || strings.Contains(out, "PROJECT") {
		t.Errorf("a project's group must not be listed outside it:\n%s", out)
	}
	if !strings.Contains(out, "standalone") {
		t.Errorf("a global secret must be listed anywhere:\n%s", out)
	}

	// --all is the escape hatch, and says which scope each entry is in.
	out, err = runCmd(t, "", "list", "--all")
	if err != nil {
		t.Fatalf("list --all: %v", err)
	}
	for _, want := range []string{"SCOPE", "warden", "project", "openai", "member of warden", "standalone", "global"} {
		if !strings.Contains(out, want) {
			t.Errorf("list --all lacks %q:\n%s", want, out)
		}
	}

	// Back inside, JSON carries the same view for scripts.
	t.Chdir(repo)
	out, err = runCmd(t, "", "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var doc struct {
		Project *struct {
			Entry string `json:"entry"`
			Keys  []struct {
				Name     string `json:"name"`
				LinkedTo string `json:"linked_to"`
			} `json:"keys"`
		} `json:"project"`
		Entries []map[string]any `json:"entries"`
		Hidden  map[string]int   `json:"hidden"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("list --json = %q (%v)", out, err)
	}
	if doc.Project == nil || doc.Project.Entry != "warden" || len(doc.Project.Keys) != 2 {
		t.Fatalf("project block = %+v", doc.Project)
	}
	if doc.Hidden["member"] != 1 || doc.Hidden["project"] != 1 {
		t.Fatalf("hidden counts = %v", doc.Hidden)
	}
}

// --no-keys is for a backend where even one read prompts.
func TestListNoKeysOpensNothing(t *testing.T) {
	testEnv(t)
	addProjectGroup(t, "warden")
	out, err := runCmd(t, "", "list", "--no-keys")
	if err != nil {
		t.Fatalf("list --no-keys: %v", err)
	}
	if !strings.Contains(out, "PROJECT") || !strings.Contains(out, "warden") {
		t.Fatalf("list --no-keys lost the project header:\n%s", out)
	}
	if strings.Contains(out, "DB_URL") {
		t.Fatalf("--no-keys still read the entry:\n%s", out)
	}
}

func TestGroupList(t *testing.T) {
	testEnv(t)
	repo := addProjectGroup(t, "warden")
	t.Chdir(t.TempDir())
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "K=v\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "loose", "--from-dotenv", src); err != nil {
		t.Fatalf("add-group: %v", err)
	}

	out, err := runCmd(t, "", "group", "list")
	if err != nil {
		t.Fatalf("group list: %v", err)
	}
	for _, want := range []string{"NAME", "LOCATION", "warden", repo, "loose"} {
		if !strings.Contains(out, want) {
			t.Errorf("group list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warden *") {
		t.Errorf("the current marker should not appear outside the repository:\n%s", out)
	}

	// Inside the repository, its group is marked and --here narrows to it.
	t.Chdir(repo)
	out, err = runCmd(t, "", "group", "list")
	if err != nil {
		t.Fatalf("group list: %v", err)
	}
	if !strings.Contains(out, "warden *") {
		t.Errorf("group list should mark the current group:\n%s", out)
	}
	out, err = runCmd(t, "", "group", "list", "--here")
	if err != nil {
		t.Fatalf("group list --here: %v", err)
	}
	if strings.Contains(out, "loose") {
		t.Errorf("--here listed another repository's group:\n%s", out)
	}

	out, err = runCmd(t, "", "group", "list", "--keys")
	if err != nil {
		t.Fatalf("group list --keys: %v", err)
	}
	if !strings.Contains(out, "DB_URL") {
		t.Errorf("--keys should list the variables:\n%s", out)
	}
	if strings.Contains(out, "postgres") {
		t.Fatalf("group list printed a value:\n%s", out)
	}
}

// A group whose repository was deleted or never cloned here must come back as
// global, not vanish from every listing.
func TestListShowsAGroupWhoseDirectoryIsGone(t *testing.T) {
	cfgPath := testEnv(t)
	gone := addProjectGroup(t, "warden")
	t.Chdir(t.TempDir())
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "warden") {
		t.Fatalf("a group with a vanished location must still be listed:\n%s", out)
	}
	// The record is kept so it can be repaired, and group list says so.
	out, err = runCmd(t, "", "group", "list")
	if err != nil {
		t.Fatalf("group list: %v", err)
	}
	if !strings.Contains(out, "(missing)") {
		t.Fatalf("group list should mark the missing location:\n%s", out)
	}
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.StaleLocations(); len(got) != 1 || got[0] != "warden" {
		t.Fatalf("StaleLocations = %v", got)
	}
}

// Approving somebody else's repository is the other way a group gets a home.
func TestAllowRecordsTheLocation(t *testing.T) {
	cfgPath := testEnv(t)
	src := filepath.Join(t.TempDir(), "fields.env")
	if err := writeFileString(src, "K=v\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "add-group", "shared", "--from-dotenv", src, "--no-project"); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	dir := projectDir(t, "entry = \"shared\"\n", "")
	if _, err := runCmd(t, "", "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["shared"].Location; got != dir {
		t.Fatalf("location = %q, want %q", got, dir)
	}
}

// An unapproved project file must not narrow the view, and must say so.
func TestUnapprovedProjectDoesNotScope(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "standalone"); err != nil {
		t.Fatalf("add: %v", err)
	}
	projectDir(t, "entry = \"someone-elses\"\n", "")

	out, status, err := runWithStderr(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(out, "PROJECT") {
		t.Fatalf("an unapproved project file scoped the listing:\n%s", out)
	}
	if !strings.Contains(status, "not approved") {
		t.Fatalf("list should say why it ignored the project file: %q", status)
	}
	if !strings.Contains(out, "standalone") {
		t.Fatalf("global entries should still be listed:\n%s", out)
	}
}

func TestInitWritesAndApprovesTheProjectFile(t *testing.T) {
	cfgPath := testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(t.TempDir(), "seed.env")
	if err := writeFileString(src, "DB_URL=postgres://test\nAPI_KEY=sk-test-api\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, err := runCmd(t, "", "init", "warden", "--from-dotenv", src,
		"--include", "openai", "--only", "DB_URL,API_KEY"); err != nil {
		t.Fatalf("init: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, vault.ProjectFileName))
	if err != nil {
		t.Fatalf("no project file: %v", err)
	}
	for _, want := range []string{`entry = "warden"`, `include = ["openai"]`, `only = ["DB_URL", "API_KEY"]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("project file lacks %q:\n%s", want, b)
		}
	}
	// Approved, so exec resolves it without a second step.
	_, proj, err := vault.LoadProject(dir)
	if err != nil || proj == nil {
		t.Fatalf("LoadProject: %v", err)
	}
	trusted, err := vault.IsProjectTrusted(filepath.Join(filepath.Dir(cfgPath), vault.TrustFileName), proj)
	if err != nil || !trusted {
		t.Fatalf("init should approve what it wrote (trusted=%v err=%v)", trusted, err)
	}
	cfg, err := vault.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Groups["warden"].Location; got != dir {
		t.Fatalf("location = %q, want %q", got, dir)
	}

	// An existing file is never replaced silently.
	if _, err := runCmd(t, "", "init", "other"); err == nil {
		t.Fatal("init overwrote an existing project file")
	}
}

// include is the direnv-style layer: global entries underneath, the project's
// own entry on top.
func TestIncludedEntriesAreMergedBeneathTheProject(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := runCmd(t, "sk-global-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}
	src := filepath.Join(t.TempDir(), "seed.env")
	if err := writeFileString(src, "DB_URL=postgres://test\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "init", "warden", "--from-dotenv", src, "--include", "openai"); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, err := runCmd(t, "", "exec", "--list", "--", "true")
	if err != nil {
		t.Fatalf("exec --list: %v", err)
	}
	for _, want := range []string{"OPENAI_API_KEY", "DB_URL"} {
		if !strings.Contains(out, want) {
			t.Errorf("exec should inject %q from the project's layers:\n%s", want, out)
		}
	}

	// The project's own entry wins on a variable both provide.
	if _, err := runCmd(t, "sk-project-value", "group", "set", "warden", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("group set: %v", err)
	}
	out, err = runCmd(t, "", "export", "--format", "dotenv")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(out, "sk-project-value") || strings.Contains(out, "sk-global-value") {
		t.Fatalf("the project entry should win over an include:\n%s", out)
	}
}

// only tracks the project's own group, so it must not narrow the entries the
// file includes — syncing it would otherwise drop them all, silently.
func TestProjectOnlyDoesNotNarrowIncludes(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := runCmd(t, "sk-test-value", "add", "openai", "--as", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("add: %v", err)
	}
	src := filepath.Join(t.TempDir(), "seed.env")
	if err := writeFileString(src, "DB_URL=postgres://test\nNOISE=x\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "init", "warden", "--from-dotenv", src,
		"--include", "openai", "--only", "DB_URL"); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, err := runCmd(t, "", "exec", "--list", "--", "true")
	if err != nil {
		t.Fatalf("exec --list: %v", err)
	}
	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Errorf("an included entry must survive the project's only list:\n%s", out)
	}
	if !strings.Contains(out, "DB_URL") {
		t.Errorf("the listed key is missing:\n%s", out)
	}
	// only still narrows the project's own group.
	if strings.Contains(out, "NOISE") {
		t.Errorf("only should narrow the project's own entry:\n%s", out)
	}

	// --only on the command line is one explicit request, and narrows all of it.
	out, err = runCmd(t, "", "exec", "--list", "--only", "DB_URL", "--", "true")
	if err != nil {
		t.Fatalf("exec --only: %v", err)
	}
	if strings.Contains(out, "OPENAI_API_KEY") {
		t.Errorf("--only must narrow everything:\n%s", out)
	}
}

// Once a project file lists its keys, sanduk keeps that list current.
func TestOnlyFollowsTheGroup(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(t.TempDir(), "seed.env")
	if err := writeFileString(src, "DB_URL=postgres://test\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "init", "warden", "--from-dotenv", src, "--only", "DB_URL"); err != nil {
		t.Fatalf("init: %v", err)
	}
	path := filepath.Join(dir, vault.ProjectFileName)

	if _, err := runCmd(t, "sk-test-value", "group", "set", "warden", "STRIPE_KEY"); err != nil {
		t.Fatalf("group set: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `only = ["DB_URL", "STRIPE_KEY"]`) {
		t.Fatalf("a new field should join only:\n%s", b)
	}
	// The comments explaining the file must survive the rewrite.
	if !strings.Contains(string(b), "# Non-secret") {
		t.Fatalf("the rewrite lost the file's comments:\n%s", b)
	}
	// And the rewrite re-approves, so exec keeps working.
	if _, err := runCmd(t, "", "exec", "--list", "--", "true"); err != nil {
		t.Fatalf("exec after the rewrite: %v", err)
	}

	if _, err := runCmd(t, "", "group", "unset", "warden", "STRIPE_KEY"); err != nil {
		t.Fatalf("group unset: %v", err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "STRIPE_KEY") {
		t.Fatalf("a removed field should leave only:\n%s", b)
	}

	// A file with no only list is never given one.
	plain := t.TempDir()
	t.Chdir(plain)
	if _, err := runCmd(t, "", "init", "plain", "--from-dotenv", src); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := runCmd(t, "sk-test-value", "group", "set", "plain", "NEW_KEY"); err != nil {
		t.Fatalf("group set: %v", err)
	}
	b, _ = os.ReadFile(filepath.Join(plain, vault.ProjectFileName))
	if strings.Contains(string(b), "only") {
		t.Fatalf("sanduk should not start an only list:\n%s", b)
	}
}

// The way into somebody else's project: their committed template names the
// keys, and sanduk asks for the values.
func TestTemplateKeys(t *testing.T) {
	dir := t.TempDir()
	if err := writeFileString(filepath.Join(dir, ".env.example"),
		"# the app needs these\nDB_URL=postgres://changeme\nAPI_KEY=\nSTRIPE_KEY=sk_test_xxx\n"); err != nil {
		t.Fatal(err)
	}

	keys, path, err := templateKeys(dir, "auto")
	if err != nil {
		t.Fatalf("templateKeys: %v", err)
	}
	if path != filepath.Join(dir, ".env.example") {
		t.Fatalf("path = %q", path)
	}
	want := []string{"API_KEY", "DB_URL", "STRIPE_KEY"}
	if len(keys) != 3 {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}

	// .env.template is just as common a spelling.
	other := t.TempDir()
	if err := writeFileString(filepath.Join(other, ".env.template"), "TOKEN=xxx\n"); err != nil {
		t.Fatal(err)
	}
	if _, path, err := templateKeys(other, "auto"); err != nil || filepath.Base(path) != ".env.template" {
		t.Fatalf("templateKeys found %q (%v)", path, err)
	}

	if _, _, err := templateKeys(t.TempDir(), "auto"); err == nil {
		t.Fatal("expected an error when there is no template")
	}
}

// Prompting needs a terminal, and says so rather than failing obscurely.
func TestInitFromTemplateNeedsATerminal(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := writeFileString(filepath.Join(dir, ".env.example"), "DB_URL=changeme\n"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "", "init", "warden", "--from-template")
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("err = %v, want a message about needing a terminal", err)
	}
	// Nothing was written: a command that cannot finish must not leave the
	// repository half set up.
	if _, statErr := os.Stat(filepath.Join(dir, vault.ProjectFileName)); !os.IsNotExist(statErr) {
		t.Fatal("a failed init wrote a project file")
	}
}

func TestInitEnvrc(t *testing.T) {
	testEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(t.TempDir(), "seed.env")
	if err := writeFileString(src, "DB_URL=postgres://test\n"); err != nil {
		t.Fatal(err)
	}

	// Off by default: direnv exports into the shell, which is the mode that
	// leaks furthest, so it is never written unasked.
	if _, err := runCmd(t, "", "init", "warden", "--from-dotenv", src); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".envrc")); !os.IsNotExist(err) {
		t.Fatal("init wrote a .envrc without being asked")
	}

	other := t.TempDir()
	t.Chdir(other)
	if _, err := runCmd(t, "", "init", "agents", "--from-dotenv", src, "--envrc"); err != nil {
		t.Fatalf("init --envrc: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(other, ".envrc"))
	if err != nil {
		t.Fatalf("no .envrc: %v", err)
	}
	got := string(b)
	for _, want := range []string{"sanduk export --format shell", "sanduk allow", "inherits"} {
		if !strings.Contains(got, want) {
			t.Errorf(".envrc lacks %q:\n%s", want, got)
		}
	}
	// It must name no value, like the project file next to it.
	if strings.Contains(got, "postgres") {
		t.Fatalf(".envrc contains a value:\n%s", got)
	}

	// An existing .envrc is somebody's work; never replaced silently.
	if _, err := runCmd(t, "", "init", "agents", "--envrc", "--force"); err != nil {
		t.Fatalf("init --envrc --force: %v", err)
	}
	if err := writeFileString(filepath.Join(other, ".envrc"), "# mine\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "init", "agents", "--envrc"); err == nil {
		t.Fatal("init replaced an existing .envrc")
	}
	b, _ = os.ReadFile(filepath.Join(other, ".envrc"))
	if string(b) != "# mine\n" {
		t.Fatalf(".envrc was overwritten:\n%s", b)
	}
}
