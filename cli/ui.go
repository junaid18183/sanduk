package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// redact returns a display-safe form of a secret: a mask plus the last four
// characters, enough to tell two keys apart without revealing one.
func redact(s string) string {
	const keep = 4
	if len(s) <= keep {
		return strings.Repeat("*", len(s))
	}
	return "****" + s[len(s)-keep:]
}

// requireTerminal refuses to print a secret anywhere but a terminal unless
// force is set, so a value cannot quietly end up in a pipe, a log or an agent's
// captured output.
func requireTerminal(w io.Writer, force bool) error {
	if force {
		return nil
	}
	if f, ok := w.(*os.File); ok && isTerminal(f) {
		return nil
	}
	return fmt.Errorf("refusing to print a secret to a non-terminal; use --force to override")
}
