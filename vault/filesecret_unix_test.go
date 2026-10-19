//go:build !windows

package vault

import (
	"os"
	"strings"
	"testing"
)

// The ephemeral path must be readable by whoever inherits the descriptor —
// this is the whole mechanism, so verify it really works on this platform
// rather than trusting that /dev/fd behaves.
func TestEphemeralPathIsReadable(t *testing.T) {
	paths, keep, err := materializeEphemeral(map[string]FilePayload{
		"GCP": {Filename: "sa.json", Data: []byte(`{"type":"service_account"}`)},
	})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	defer func() {
		for _, f := range keep {
			f.Close()
		}
	}()

	path := paths["GCP"]
	if !strings.HasPrefix(path, "/dev/fd/") {
		t.Fatalf("path = %q, want /dev/fd/N", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != `{"type":"service_account"}` {
		t.Fatalf("contents = %q", got)
	}
}

// Nothing readable may be left in the temp directory.
func TestEphemeralFileIsUnlinked(t *testing.T) {
	paths, keep, err := materializeEphemeral(map[string]FilePayload{
		"GCP": {Data: []byte("sk-test-value")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, f := range keep {
			f.Close()
		}
	}()

	// The descriptor's backing file has no name: its temp path is gone.
	if name := keep[0].Name(); name != "" {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("%s still exists on disk; the secret was not unlinked", name)
		}
	}
	_ = paths
}

// Once the descriptors close, the path stops resolving — which is what makes
// the data disappear when the child exits.
func TestEphemeralPathDiesWithTheDescriptor(t *testing.T) {
	paths, keep, err := materializeEphemeral(map[string]FilePayload{
		"GCP": {Data: []byte("sk-test-value")},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := paths["GCP"]
	for _, f := range keep {
		f.Close()
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Fatalf("%s still readable after the descriptor closed", path)
	}
}

func TestEphemeralHandlesSeveralPayloads(t *testing.T) {
	paths, keep, err := materializeEphemeral(map[string]FilePayload{
		"ONE": {Data: []byte("first")},
		"TWO": {Data: []byte("second")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, f := range keep {
			f.Close()
		}
	}()
	if paths["ONE"] == paths["TWO"] {
		t.Fatal("both payloads got the same descriptor")
	}
	for name, want := range map[string]string{"ONE": "first", "TWO": "second"} {
		got, err := os.ReadFile(paths[name])
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q (%v), want %q", name, got, err, want)
		}
	}
}
