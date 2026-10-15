package vault

import (
	"bufio"
	"fmt"
	"strings"
)

// ParseDotenv parses the contents of a .env file into key/value pairs.
//
// Supported: blank lines, `#` comments, an optional `export ` prefix, and
// single- or double-quoted values.
func ParseDotenv(b []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")

		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", line)
		}
		out[key] = unquote(strings.TrimSpace(value))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func unquote(v string) string {
	if len(v) < 2 {
		return v
	}
	switch {
	case v[0] == '"' && v[len(v)-1] == '"':
		inner := v[1 : len(v)-1]
		r := strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\"`, `"`, `\\`, `\`)
		return r.Replace(inner)
	case v[0] == '\'' && v[len(v)-1] == '\'':
		return v[1 : len(v)-1]
	}
	return v
}
