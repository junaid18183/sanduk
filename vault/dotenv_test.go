package vault

import "testing"

func TestParseDotenv(t *testing.T) {
	in := []byte(`
# a comment

export OPENAI_API_KEY=sk-abc
PLAIN=value
SPACED="a b c"
SINGLE='quoted'
ESCAPED="line\nbreak"
EMPTY=
`)
	got, err := ParseDotenv(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]string{
		"OPENAI_API_KEY": "sk-abc",
		"PLAIN":          "value",
		"SPACED":         "a b c",
		"SINGLE":         "quoted",
		"ESCAPED":        "line\nbreak",
		"EMPTY":          "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseDotenvInvalidLine(t *testing.T) {
	if _, err := ParseDotenv([]byte("NOT_A_PAIR\n")); err == nil {
		t.Fatal("expected error for a line without '='")
	}
}
