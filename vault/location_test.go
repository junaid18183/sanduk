package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGroupLocationAndLookup(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	inner := filepath.Join(repo, "services", "api")
	sibling := filepath.Join(root, "repo-old")
	for _, d := range []string{inner, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	c := &Config{Groups: map[string]GroupMeta{"warden": {}, "loose": {}}}
	if !c.SetGroupLocation("warden", repo) {
		t.Fatal("SetGroupLocation should record a group that exists")
	}
	if c.SetGroupLocation("absent", repo) {
		t.Fatal("SetGroupLocation should refuse an entry with no group record")
	}
	if got := c.Groups["warden"].Location; got != repo {
		t.Fatalf("location = %q, want %q", got, repo)
	}

	if name, ok := c.GroupAt(repo); !ok || name != "warden" {
		t.Fatalf("GroupAt(repo) = (%q, %v), want warden", name, ok)
	}
	// A group reaches everything below its directory, the way a project file
	// found by walking up does.
	if name, ok := c.GroupAt(inner); !ok || name != "warden" {
		t.Fatalf("GroupAt(subdir) = (%q, %v), want warden", name, ok)
	}
	// A sibling whose name merely starts with the location is a different
	// directory.
	if name, ok := c.GroupAt(sibling); ok {
		t.Fatalf("GroupAt(%s) = %q, want no match", sibling, name)
	}
	if name, ok := c.GroupAt(root); ok {
		t.Fatalf("GroupAt(parent) = %q, want no match", name)
	}
	if got := c.GroupLocations(); !reflect.DeepEqual(got, map[string]string{"warden": repo}) {
		t.Fatalf("GroupLocations = %v", got)
	}

	// A repository checked out inside another keeps its own group.
	if !c.SetGroupLocation("loose", inner) {
		t.Fatal("SetGroupLocation(loose)")
	}
	if name, _ := c.GroupAt(inner); name != "loose" {
		t.Fatalf("GroupAt(inner) = %q, want the deepest location to win", name)
	}
	if name, _ := c.GroupAt(repo); name != "warden" {
		t.Fatalf("GroupAt(repo) = %q, want warden", name)
	}

	if !c.SetGroupLocation("warden", "") {
		t.Fatal("SetGroupLocation should clear")
	}
	if name, ok := c.GroupAt(repo); ok {
		t.Fatalf("GroupAt after clearing = %q, want no match", name)
	}
}

// A group whose directory is gone must not disappear from every listing: no
// working directory can be inside a path that does not exist, so it would be
// unreachable. It goes back to being global instead.
func TestVanishedLocationIsUnclaimed(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "deleted-repo")
	here := t.TempDir()
	c := &Config{Groups: map[string]GroupMeta{"ghost": {}, "live": {}}}
	if !c.SetGroupLocation("ghost", gone) || !c.SetGroupLocation("live", here) {
		t.Fatal("SetGroupLocation")
	}

	if got := c.GroupLocations(); len(got) != 1 || got["live"] != here {
		t.Fatalf("GroupLocations = %v, want only the directory that exists", got)
	}
	if name, ok := c.GroupAt(gone); ok {
		t.Fatalf("GroupAt(%s) = %q, want no match", gone, name)
	}
	// The record itself is kept, and reported, so the user can repair it.
	if got := c.Groups["ghost"].Location; got != gone {
		t.Fatalf("location was dropped: %q", got)
	}
	if got := c.StaleLocations(); len(got) != 1 || got[0] != "ghost" {
		t.Fatalf("StaleLocations = %v, want [ghost]", got)
	}

	// A file at the path is not a directory a repository could live in.
	asFile := filepath.Join(t.TempDir(), "file")
	if err := WriteFileAtomic(asFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !c.SetGroupLocation("ghost", asFile) {
		t.Fatal("SetGroupLocation")
	}
	if _, ok := c.GroupLocations()["ghost"]; ok {
		t.Fatal("a file must not count as a location")
	}
}

func TestLinkedByMirror(t *testing.T) {
	c := &Config{
		Secrets: map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY"}},
		Groups:  map[string]GroupMeta{"warden": {}, "agents": {}},
	}

	c.RecordLink("openai", "warden")
	c.RecordLink("openai", "agents")
	c.RecordLink("openai", "warden") // repeated links must not pile up
	if got := c.Secrets["openai"].LinkedBy; !reflect.DeepEqual(got, []string{"agents", "warden"}) {
		t.Fatalf("linked_by = %v, want sorted and deduplicated", got)
	}
	// A target with no record yet gets one: only a flat secret can be linked,
	// so its type is already known.
	c.RecordLink("github", "warden")
	if got := c.Secrets["github"].LinkedBy; !reflect.DeepEqual(got, []string{"warden"}) {
		t.Fatalf("linked_by for a new target = %v", got)
	}

	c.RemoveLink("openai", "warden")
	if got := c.Secrets["openai"].LinkedBy; !reflect.DeepEqual(got, []string{"agents"}) {
		t.Fatalf("after RemoveLink = %v", got)
	}
	// An emptied list is dropped rather than written as [].
	c.RemoveLink("openai", "agents")
	if got := c.Secrets["openai"].LinkedBy; got != nil {
		t.Fatalf("emptied linked_by = %v, want nil", got)
	}

	// Forgetting a group drops the hints naming it, since a removed group
	// owns nothing.
	c.RecordLink("openai", "warden")
	c.Forget("warden")
	if got := c.Secrets["openai"].LinkedBy; got != nil {
		t.Fatalf("after forgetting the group = %v, want nil", got)
	}

	// SetLinkedBy replaces the whole mirror with the backend's own graph.
	c.RecordLink("openai", "stale")
	c.SetLinkedBy(map[string][]string{"github": {"agents"}})
	if got := c.Secrets["openai"].LinkedBy; got != nil {
		t.Fatalf("openai kept a stale hint: %v", got)
	}
	if got := c.Secrets["github"].LinkedBy; !reflect.DeepEqual(got, []string{"agents"}) {
		t.Fatalf("github linked_by = %v", got)
	}
}

// The mirror is metadata: it must survive a round trip through the file, and
// it must never carry anything but names.
func TestLinkedByAndLocationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	in := &Config{
		Secrets: map[string]SecretMeta{"openai": {Env: "OPENAI_API_KEY", LinkedBy: []string{"warden"}}},
		Groups:  map[string]GroupMeta{"warden": {Description: "d", Location: "/repo"}},
	}
	if err := SaveConfig(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Groups["warden"].Location; got != "/repo" {
		t.Fatalf("location did not round-trip: %q", got)
	}
	if got := out.Secrets["openai"].LinkedBy; !reflect.DeepEqual(got, []string{"warden"}) {
		t.Fatalf("linked_by did not round-trip: %v", got)
	}
}
