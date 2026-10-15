package cli

import (
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
	got, err := parseFieldFlags([]string{"A=1", "B=two=three"})
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "1" || got["B"] != "two=three" {
		t.Fatalf("unexpected fields: %v", got)
	}
	if _, err := parseFieldFlags([]string{"NOEQUALS"}); err == nil {
		t.Fatal("expected error for a field without '='")
	}
}
