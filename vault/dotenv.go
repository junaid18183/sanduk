package vault

import (
	"fmt"
	"strings"
)

// ParseDotenv parses the contents of a .env file into key/value pairs.
//
// Supported: blank lines, `#` comments, an optional `export ` prefix, unquoted
// values with a trailing inline comment (`KEY=value # note`), and single- or
// double-quoted values. A quoted value may span several lines, so a PEM key or
// any other multi-line secret survives a round trip.
//
// Double-quoted values understand the escapes \n, \r, \t, \" and \\; single-quoted
// values are taken literally, matching common dotenv parsers.
func ParseDotenv(b []byte) (map[string]string, error) {
	out := make(map[string]string)
	lines := splitDotenvLines(string(b))

	for i := 0; i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")

		key, rest, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", i+1)
		}

		value, consumed, err := readDotenvValue(rest, lines[i+1:])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		i += consumed
		out[key] = value
	}
	return out, nil
}

// splitDotenvLines normalises CRLF and splits on LF. It does not drop trailing
// empty lines, so line numbers in errors stay accurate.
func splitDotenvLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

// readDotenvValue parses the value part of a KEY=VALUE line. For a quoted value
// that is not closed on the same line it pulls in following lines until the
// closing quote, returning how many extra lines it consumed.
func readDotenvValue(rest string, following []string) (value string, consumed int, err error) {
	v := strings.TrimLeft(rest, " \t")
	if v == "" {
		return "", 0, nil
	}
	switch v[0] {
	case '"', '\'':
		return readQuotedValue(v, following)
	default:
		return stripInlineComment(v), 0, nil
	}
}

// stripInlineComment removes a trailing `# comment` from an unquoted value. The
// `#` only starts a comment when it is preceded by whitespace, so a value like
// `abc#def` is left intact.
func stripInlineComment(v string) string {
	for i := 0; i < len(v); i++ {
		if v[i] == '#' && (i == 0 || v[i-1] == ' ' || v[i-1] == '\t') {
			v = v[:i]
			break
		}
	}
	return strings.TrimRight(v, " \t")
}

func readQuotedValue(v string, following []string) (string, int, error) {
	quote := v[0]
	var sb strings.Builder
	body := v[1:]
	consumed := 0

	for {
		if end, ok := findClosingQuote(body, quote); ok {
			sb.WriteString(body[:end])
			full := sb.String()
			if quote == '"' {
				full = unescapeDoubleQuoted(full)
			}
			tail := strings.TrimSpace(body[end+1:])
			if tail != "" && !strings.HasPrefix(tail, "#") {
				return "", 0, fmt.Errorf("unexpected text after closing quote: %q", tail)
			}
			return full, consumed, nil
		}

		sb.WriteString(body)
		sb.WriteString("\n")
		if consumed >= len(following) {
			return "", 0, fmt.Errorf("unterminated %c-quoted value", quote)
		}
		body = following[consumed]
		consumed++
	}
}

// findClosingQuote locates the closing quote, honouring backslash escapes inside
// a double-quoted value so `\"` does not end the string.
func findClosingQuote(s string, quote byte) (int, bool) {
	for i := 0; i < len(s); i++ {
		if quote == '"' && s[i] == '\\' {
			i++ // skip the escaped character
			continue
		}
		if s[i] == quote {
			return i, true
		}
	}
	return 0, false
}

func unescapeDoubleQuoted(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\t`, "\t", `\"`, `"`, `\\`, `\`)
	return r.Replace(s)
}
