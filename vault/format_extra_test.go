package vault

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatEnvDotenvSingleQuotesDollarAndHash(t *testing.T) {
	var buf bytes.Buffer
	env := map[string]string{"DOLLAR": "pa$word", "HASH": "a#b"}
	if err := FormatEnv(&buf, env, FormatDotenv); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "DOLLAR='pa$word'\n") {
		t.Fatalf("a $ value must be single-quoted, got %q", out)
	}
	if !strings.Contains(out, "HASH='a#b'\n") {
		t.Fatalf("a # value must be single-quoted, got %q", out)
	}
}
