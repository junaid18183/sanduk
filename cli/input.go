package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// readSecret resolves a secret value from the selected source:
//
//	--from-env  -> the named environment variable
//	--from-file -> the named file (trailing newline trimmed)
//	non-TTY in  -> all of stdin
//	TTY         -> an echo-disabled prompt (confirmed when confirm is true)
//
// Values are never accepted as command-line arguments, so they cannot leak
// through process listings.
func readSecret(fromEnv, fromFile string, confirm bool) (string, error) {
	switch {
	case fromEnv != "":
		v, ok := os.LookupEnv(fromEnv)
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", fromEnv)
		}
		if v == "" {
			return "", fmt.Errorf("environment variable %s is empty", fromEnv)
		}
		return v, nil

	case fromFile != "":
		b, err := os.ReadFile(fromFile)
		if err != nil {
			return "", err
		}
		v := strings.TrimRight(string(b), "\r\n")
		if v == "" {
			return "", fmt.Errorf("file %s is empty", fromFile)
		}
		return v, nil
	}

	if !isTerminal(os.Stdin) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		v := strings.TrimRight(string(b), "\r\n")
		if v == "" {
			return "", fmt.Errorf("no value provided on stdin")
		}
		return v, nil
	}

	v, err := promptSecret("Value: ")
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("no value provided")
	}
	if confirm {
		again, err := promptSecret("Confirm value: ")
		if err != nil {
			return "", err
		}
		if again != v {
			return "", fmt.Errorf("values do not match")
		}
	}
	return v, nil
}

// promptSecret reads a line from the terminal with echo disabled.
func promptSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// promptLine reads an echoed line from the terminal.
func promptLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				sb.WriteByte(buf[0])
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	fmt.Fprintln(os.Stderr)
	return sb.String(), nil
}
