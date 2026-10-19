package cli

import "testing"

func TestRedactNeverRevealsSecret(t *testing.T) {
	got := redact("sk-abcdefgh1234")
	if got == "sk-abcdefgh1234" {
		t.Fatal("redact returned the raw secret")
	}
	if want := "****1234"; got != want {
		t.Fatalf("redact = %q, want %q", got, want)
	}
	if short := redact("ab"); short != "**" {
		t.Fatalf("redact short = %q, want **", short)
	}
}
