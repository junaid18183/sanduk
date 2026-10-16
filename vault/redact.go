package vault

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// IsTerminal reports whether f is attached to a terminal. It is used to gate
// commands that would otherwise dump raw secrets into a pipe.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Redact returns a display-safe representation of a secret: a mask plus the
// last four characters, enough to tell two keys apart without revealing one.
func Redact(s string) string {
	const keep = 4
	if len(s) <= keep {
		return strings.Repeat("*", len(s))
	}
	return "****" + s[len(s)-keep:]
}
