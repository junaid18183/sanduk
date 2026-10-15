package vault

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Env formats are accepted by FormatEnv.
const (
	FormatShell  = "shell"
	FormatDotenv = "dotenv"
	FormatJSON   = "json"
)

// FormatEnv writes env to w in the requested format.
//
//	shell  -> export VAR='value'
//	dotenv -> VAR=value
//	json   -> {"VAR": "value"}
func FormatEnv(w io.Writer, env map[string]string, format string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	switch format {
	case FormatShell:
		for _, k := range keys {
			fmt.Fprintf(w, "export %s=%s\n", k, shellQuote(env[k]))
		}
	case FormatDotenv:
		for _, k := range keys {
			fmt.Fprintf(w, "%s=%s\n", k, dotenvQuote(env[k]))
		}
	case FormatJSON:
		out := make(map[string]string, len(env))
		for k, v := range env {
			out[k] = v
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		fmt.Fprintln(w)
	default:
		return fmt.Errorf("unknown format %q (want shell, dotenv or json)", format)
	}
	return nil
}

// shellQuote wraps v in single quotes, escaping embedded single quotes. Always
// quoting avoids any shell interpretation of the secret.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// dotenvQuote emits v bare when it is unambiguous, otherwise double-quoted with
// the standard escapes understood by most dotenv parsers.
func dotenvQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\n\r\"'\\$#") {
		return v
	}
	r := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
	)
	return "\"" + r.Replace(v) + "\""
}
