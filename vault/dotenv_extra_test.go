package vault

import (
	"testing"
)

func TestParseDotenvInlineComment(t *testing.T) {
	got, err := ParseDotenv([]byte("KEY=abc123 # prod key\nHASH=abc#def\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["KEY"] != "abc123" {
		t.Fatalf("KEY = %q, want the comment stripped", got["KEY"])
	}
	// A '#' not preceded by whitespace is part of the value.
	if got["HASH"] != "abc#def" {
		t.Fatalf("HASH = %q, want it preserved", got["HASH"])
	}
}

func TestParseDotenvQuoteKeepsHash(t *testing.T) {
	got, err := ParseDotenv([]byte("KEY=\"a # b\"\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["KEY"] != "a # b" {
		t.Fatalf("KEY = %q, want the quoted comment preserved", got["KEY"])
	}
}

func TestParseDotenvMultilineQuoted(t *testing.T) {
	const pem = "-----BEGIN KEY-----\nMIIB\n-----END KEY-----"
	in := "PRIVATE_KEY=\"" + pem + "\"\nNEXT=1\n"
	got, err := ParseDotenv([]byte(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["PRIVATE_KEY"] != pem {
		t.Fatalf("PRIVATE_KEY = %q, want %q", got["PRIVATE_KEY"], pem)
	}
	if got["NEXT"] != "1" {
		t.Fatalf("NEXT = %q, want 1 (the parser must resume after the closing quote)", got["NEXT"])
	}
}

func TestParseDotenvUnterminatedQuote(t *testing.T) {
	if _, err := ParseDotenv([]byte("KEY=\"unterminated\n")); err == nil {
		t.Fatal("expected an error for an unterminated quoted value")
	}
}
