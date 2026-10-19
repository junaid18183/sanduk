package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddFileRoundTripsBytes(t *testing.T) {
	v := newFileVault(t)
	// Binary, including a NUL, to prove base64 round-tripping.
	data := []byte{0x00, 0x01, 'k', 'e', 'y', 0xff}
	if err := v.AddFile("gcp", data, "sa.json"); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	e, err := v.Get("gcp")
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != TypeFile || string(e.Data) != string(data) || e.Filename != "sa.json" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestAddFileRefusesEmpty(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddFile("gcp", nil, ""); err == nil {
		t.Fatal("an empty file was stored")
	}
}

func TestFileEntryMustCarryOnlyData(t *testing.T) {
	for name, e := range map[string]*Entry{
		"with value":  {Version: 1, Type: TypeFile, Data: []byte("x"), Value: "v"},
		"with fields": {Version: 1, Type: TypeFile, Data: []byte("x"), Fields: map[string]string{"A": "b"}},
		"no data":     {Version: 1, Type: TypeFile},
		"secret+data": {Version: 1, Type: TypeSecret, Value: "v", Data: []byte("x")},
		"group+data":  {Version: 1, Type: TypeGroup, Fields: map[string]string{"A": "b"}, Data: []byte("x")},
	} {
		if _, err := e.Marshal(); err == nil {
			t.Fatalf("%s: expected a rejection", name)
		}
	}
}

func TestResolveEntryYieldsAPayload(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddFile("gcp", []byte("{}"), "sa.json"); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	cfg.RecordFile("gcp", "GOOGLE_APPLICATION_CREDENTIALS", "")

	env, files, err := v.ResolveEntry("gcp", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 {
		t.Fatalf("env = %v, want none", env)
	}
	p, ok := files["GOOGLE_APPLICATION_CREDENTIALS"]
	if !ok || string(p.Data) != "{}" || p.Filename != "sa.json" {
		t.Fatalf("files = %v", files)
	}
}

// ResolveEnv is the variables-only view; a file secret has no value to return.
func TestResolveEnvRefusesAFileSecret(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddFile("gcp", []byte("{}"), ""); err != nil {
		t.Fatal(err)
	}
	_, err := v.ResolveEnv("gcp", &Config{})
	if err == nil || !strings.Contains(err.Error(), "file secret") {
		t.Fatalf("err = %v", err)
	}
}

// A filename out of the vault must not escape the directory sanduk chose.
func TestSafeFilenameStripsPaths(t *testing.T) {
	for _, hint := range []string{"../../id_rsa", "/etc/passwd", "..", ".", "", "   "} {
		got := SafeFilename("GCP_KEY", FilePayload{Filename: hint})
		if strings.ContainsAny(got, `/\`) || got == ".." || got == "." {
			t.Fatalf("SafeFilename(%q) = %q, which is not a plain basename", hint, got)
		}
	}
	if got := SafeFilename("GCP_KEY", FilePayload{Filename: "sa.json"}); got != "sa.json" {
		t.Fatalf("got %q", got)
	}
	if got := SafeFilename("GCP_KEY", FilePayload{}); got != "gcp_key" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestMaterializeIntoWritesPrivateFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "creds")
	paths, err := MaterializeInto(dir, map[string]FilePayload{
		"GCP": {Filename: "sa.json", Data: []byte("{}")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(paths["GCP"])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{}" {
		t.Fatalf("contents = %q", got)
	}
	info, err := os.Stat(paths["GCP"])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
	if filepath.Base(paths["GCP"]) != "sa.json" {
		t.Fatalf("path = %q", paths["GCP"])
	}
}

// Two payloads sharing a filename hint must not overwrite each other.
func TestMaterializeIntoAvoidsCollisions(t *testing.T) {
	dir := t.TempDir()
	paths, err := MaterializeInto(dir, map[string]FilePayload{
		"ONE": {Filename: "key.json", Data: []byte("first")},
		"TWO": {Filename: "key.json", Data: []byte("second")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if paths["ONE"] == paths["TWO"] {
		t.Fatalf("both payloads landed at %q", paths["ONE"])
	}
	for name, want := range map[string]string{"ONE": "first", "TWO": "second"} {
		got, err := os.ReadFile(paths[name])
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q (%v), want %q", name, got, err, want)
		}
	}
}
