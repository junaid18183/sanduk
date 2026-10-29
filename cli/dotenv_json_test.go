package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// dotenvAsJSON writes a .env file's pairs to a JSON object file and returns its
// path. add-group reads JSON, not .env, so tests that seeded a group from a .env
// file now seed it through --from-json with the same fields.
func dotenvAsJSON(t *testing.T, src string) string {
	t.Helper()
	fields, err := readDotenvFile(src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "fields.json")
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}
