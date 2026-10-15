package vault

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatEnvShellQuotes(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"KEY": "it's a $ecret"}
	if err := FormatEnv(&buf, env, FormatShell); err != nil {
		t.Fatal(err)
	}
	want := `export KEY='it'\''s a $ecret'` + "\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestFormatEnvDotenvBareAndQuoted(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"SAFE": "abc-123", "SPACED": "a b"}
	if err := FormatEnv(&buf, env, FormatDotenv); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "SAFE=abc-123\n") {
		t.Fatalf("expected bare value, got %q", out)
	}
	if !strings.Contains(out, `SPACED="a b"`+"\n") {
		t.Fatalf("expected quoted value, got %q", out)
	}
}

func TestFormatEnvJSONRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"A": "1", "B": "two"}
	if err := FormatEnv(&buf, env, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["A"] != "1" || got["B"] != "two" {
		t.Fatalf("unexpected JSON: %v", got)
	}
}

func TestFormatEnvUnknownFormat(t *testing.T) {
	if err := FormatEnv(&bytes.Buffer{}, nil, "yaml"); err == nil {
		t.Fatal("expected error for unknown format")
	}
}
