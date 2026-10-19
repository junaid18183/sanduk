package vault

// Leak scanning: look for copies of stored values in places they should never
// be — the working tree, git history, shell history.
//
// Nothing here ever reports a value. A match is an entry name and a location,
// because the whole point is to be safe to run and safe to paste. The needles
// are held in memory only, and are never passed to another program as an
// argument: argv is world-readable, so grepping for a secret with an external
// tool would leak it to every process on the machine. That is why callers pipe
// content in and match here.

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// DefaultScanMinLength is the shortest value worth searching for. Very short
// secrets match everywhere and would bury the real findings; a credential
// below this length is reported as unscannable rather than silently skipped.
const DefaultScanMinLength = 8

// Match is one place a stored value was found.
type Match struct {
	Entry string // the vault entry whose value appeared
	Where string // file path, or a description such as "git history"
	Line  int    // 1-indexed, 0 when the source has no line numbering
	Note  string // extra context, e.g. the commit a line came from
}

// Scanner searches text for copies of stored values.
type Scanner struct {
	// byValue maps a secret value to the entries that hold it. Two entries
	// with the same value are both reported, since either could be the leak.
	byValue map[string][]string
	// Skipped names the entries whose values were too short to search for.
	Skipped []string
}

// NewScanner indexes the values to look for. values maps a label (an entry
// name, or "entry.FIELD" for a group field) to its secret.
func NewScanner(values map[string]string, minLength int) *Scanner {
	if minLength <= 0 {
		minLength = DefaultScanMinLength
	}
	s := &Scanner{byValue: map[string][]string{}}
	for label, val := range values {
		if len(val) < minLength {
			s.Skipped = append(s.Skipped, label)
			continue
		}
		s.byValue[val] = append(s.byValue[val], label)
	}
	for _, labels := range s.byValue {
		sort.Strings(labels)
	}
	sort.Strings(s.Skipped)
	return s
}

// Needles reports how many distinct values the scanner will look for.
func (s *Scanner) Needles() int { return len(s.byValue) }

// maxScanLine caps how much of a very long line is examined. A minified
// bundle or a base64 blob on one line would otherwise make scanning quadratic
// for no benefit.
const maxScanLine = 1 << 20

// ScanReader reads r line by line and returns every stored value found. A line
// containing a NUL byte means binary content: the rest of the reader is
// skipped, and ok is false so the caller can say it was not searched.
func (s *Scanner) ScanReader(where string, r io.Reader) (matches []Match, ok bool, err error) {
	if len(s.byValue) == 0 {
		return nil, true, nil
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanLine)

	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.IndexByte(text, 0) >= 0 {
			return matches, false, nil
		}
		matches = append(matches, s.scanLine(where, text, line)...)
	}
	if err := sc.Err(); err != nil {
		// A line longer than the cap is not an error worth failing on; the
		// file simply could not be searched in full.
		if strings.Contains(err.Error(), "token too long") {
			return matches, false, nil
		}
		return matches, false, fmt.Errorf("scanning %s: %w", where, err)
	}
	return matches, true, nil
}

func (s *Scanner) scanLine(where, text string, line int) []Match {
	var out []Match
	for val, labels := range s.byValue {
		if !strings.Contains(text, val) {
			continue
		}
		for _, label := range labels {
			out = append(out, Match{Entry: label, Where: where, Line: line})
		}
	}
	return out
}

// SortMatches orders findings by entry, then location, so two runs over an
// unchanged tree print the same report.
func SortMatches(m []Match) {
	sort.SliceStable(m, func(i, j int) bool {
		if m[i].Entry != m[j].Entry {
			return m[i].Entry < m[j].Entry
		}
		if m[i].Where != m[j].Where {
			return m[i].Where < m[j].Where
		}
		return m[i].Line < m[j].Line
	})
}

// ScanValues collects every value in the vault, labelled by entry (and field,
// for a group). This is the one place sanduk reads the whole vault, and the
// caller is expected to have said so out loud first.
//
// File secrets are included as whole blobs: an exact copy of a credential file
// committed to a repository is precisely what this is looking for. Group links
// are skipped, because the flat secret they point at is already in the set.
func (v *Vault) ScanValues() (map[string]string, error) {
	names, err := v.List()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, name := range names {
		e, err := v.Get(name)
		if err != nil {
			return nil, err
		}
		switch e.Type {
		case TypeSecret:
			out[name] = e.Value
		case TypeFile:
			out[name] = string(e.Data)
		case TypeGroup:
			for _, k := range sortedKeys(e.Fields) {
				out[name+"."+k] = e.Fields[k]
			}
		}
	}
	return out, nil
}
