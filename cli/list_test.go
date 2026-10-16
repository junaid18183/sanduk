package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/junaid18183/sanduk/vault"
)

func TestDescribeUsesMetadataOnly(t *testing.T) {
	cfg := &vault.Config{
		Secrets: map[string]vault.SecretMeta{
			"openai": {Env: "OPENAI_API_KEY", Description: "OpenAI key"},
		},
		Groups: map[string]vault.GroupMeta{
			"aws-prod": {Description: "AWS production"},
			"untitled": {},
		},
	}

	typ, info := describe(cfg, "openai")
	if typ != "secret" {
		t.Fatalf("openai type = %q, want secret", typ)
	}
	if info != "OPENAI_API_KEY  (OpenAI key)" {
		t.Fatalf("openai info = %q", info)
	}

	typ, info = describe(cfg, "aws-prod")
	if typ != "group" || info != "AWS production" {
		t.Fatalf("aws-prod = (%q, %q)", typ, info)
	}

	// A group without a description must still be reported as a group.
	typ, info = describe(cfg, "untitled")
	if typ != "group" || info != "-" {
		t.Fatalf("untitled = (%q, %q)", typ, info)
	}

	// An entry with no metadata defaults to a secret named after itself.
	typ, info = describe(cfg, "deepseek")
	if typ != "secret" || info != "DEEPSEEK" {
		t.Fatalf("deepseek = (%q, %q)", typ, info)
	}
}

func TestParseFieldFlags(t *testing.T) {
	got, err := parseFieldFlags([]string{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("unexpected fields: %v", got)
	}
	// A value on the command line is visible in ps; it must be refused, and
	// the error must not echo it.
	_, err = parseFieldFlags([]string{"A=sk-test-value"})
	if err == nil {
		t.Fatal("expected an error for --field KEY=VALUE")
	}
	if strings.Contains(err.Error(), "sk-test-value") {
		t.Fatalf("error leaks the value: %v", err)
	}
}

func TestReadJSONSourceRefusesInline(t *testing.T) {
	if _, err := readJSONSource(`{"A":"sk-test-value"}`); err == nil {
		t.Fatal("inline JSON must be refused")
	}
	path := filepath.Join(t.TempDir(), "f.json")
	if err := writeFileString(path, `{"A":"1"}`); err != nil {
		t.Fatal(err)
	}
	got, err := readJSONSource(path)
	if err != nil || got["A"] != "1" {
		t.Fatalf("readJSONSource(file) = %v, %v", got, err)
	}
}
