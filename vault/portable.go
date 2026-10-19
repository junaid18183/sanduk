package vault

// Importers for the export files other credential stores produce. They turn a
// file into plain NAME=value pairs, which is all the rest of the import path
// understands — the same shape ParseDotenv yields, so nothing downstream has
// to know where a secret came from.
//
// These files are plaintext dumps of a whole vault. Nothing here prints or
// logs a value, and the caller is expected to remove the source file
// afterwards (`import --delete`).

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// Import formats accepted by --format. Named apart from the output formats in
// format.go: these describe a file being read, those a stream being written.
const (
	ImportDotenv = "dotenv"
	ImportCSV    = "csv"
	Import1PUX   = "1pux"
)

// ImportFormats lists the accepted --format values, for help text and errors.
var ImportFormats = []string{ImportDotenv, ImportCSV, Import1PUX}

// DetectImportFormat guesses a file's format from its extension, defaulting to
// dotenv — a .env file is often named .env.production or similar, so an
// unrecognised extension is far more likely to be a dotenv file than anything
// else.
func DetectImportFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return ImportCSV
	case ".1pux":
		return Import1PUX
	default:
		return ImportDotenv
	}
}

// csvNameColumns and csvValueColumns are the header names other exporters use,
// lower-cased. Value columns are tried in order, so a row carrying both a
// password and a generic "value" prefers the password.
var (
	csvNameColumns  = []string{"name", "title", "item", "entry", "key"}
	csvValueColumns = []string{
		"login_password", "password", "credential", "secret",
		"api_key", "apikey", "token", "value",
	}
)

// ParseCSVExport reads a credential export in CSV form. The header decides
// which column holds the entry name and which holds the secret, so one parser
// covers the exports that differ only in column naming. A "fields" column of
// "label: value" lines (custom fields) contributes one entry per line, which
// is where API keys usually live rather than in the password column.
func ParseCSVExport(r io.Reader) (map[string]string, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1 // exports vary; tolerate ragged rows
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	index := map[string]int{}
	for i, h := range rows[0] {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}
	nameCol, ok := firstColumn(index, csvNameColumns)
	if !ok {
		return nil, fmt.Errorf("CSV has no name column (looked for %s)",
			strings.Join(csvNameColumns, ", "))
	}
	valueCol, hasValue := firstColumn(index, csvValueColumns)
	fieldsCol, hasFields := index["fields"]
	if !hasValue && !hasFields {
		return nil, fmt.Errorf("CSV has no value column (looked for %s)",
			strings.Join(csvValueColumns, ", "))
	}

	out := map[string]string{}
	for _, row := range rows[1:] {
		title := cell(row, nameCol)
		if title == "" {
			continue
		}
		if hasValue {
			if val := cell(row, valueCol); val != "" {
				addCredential(out, val, title)
			}
		}
		if hasFields {
			for label, val := range parseLabelledFields(cell(row, fieldsCol)) {
				addCredential(out, val, title, label)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no credentials found in the CSV")
	}
	return out, nil
}

// firstColumn returns the index of the first header present from names.
func firstColumn(index map[string]int, names []string) (int, bool) {
	for _, n := range names {
		if i, ok := index[n]; ok {
			return i, true
		}
	}
	return 0, false
}

func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// parseLabelledFields reads the "label: value" lines some exporters pack into
// a single custom-fields cell. A line without a colon is skipped rather than
// guessed at.
func parseLabelledFields(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		label, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		label, val = strings.TrimSpace(label), strings.TrimSpace(val)
		if label != "" && val != "" {
			out[label] = val
		}
	}
	return out
}

// pux mirrors the parts of a 1PUX export.data this importer reads. Fields it
// does not need are left out: the file also contains usernames, notes, card
// numbers and documents, and nothing here should carry those into the vault.
type pux struct {
	Accounts []struct {
		Vaults []struct {
			Items []puxItem `json:"items"`
		} `json:"vaults"`
	} `json:"accounts"`
}

type puxItem struct {
	State    string `json:"state"`
	Overview struct {
		Title string `json:"title"`
	} `json:"overview"`
	Details struct {
		LoginFields []struct {
			Value       string `json:"value"`
			Designation string `json:"designation"`
		} `json:"loginFields"`
		Sections []struct {
			Fields []struct {
				Title string          `json:"title"`
				ID    string          `json:"id"`
				Value json.RawMessage `json:"value"`
			} `json:"fields"`
		} `json:"sections"`
	} `json:"details"`
}

// Parse1PUX reads a 1Password Unencrypted Export archive. Only secret-bearing
// fields are taken: a login's password, and section fields whose value is
// concealed. Usernames, URLs, notes and attachments are deliberately ignored —
// a credential store should not absorb an address book.
func Parse1PUX(path string) (map[string]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer zr.Close()

	var data *zip.File
	for _, f := range zr.File {
		if f.Name == "export.data" {
			data = f
			break
		}
	}
	if data == nil {
		return nil, fmt.Errorf("%s has no export.data; is it a 1PUX archive?", path)
	}

	rc, err := data.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var doc pux
	if err := json.Unmarshal(raw, &doc); err != nil {
		// The decoder quotes the input on failure, and that input is secret.
		return nil, fmt.Errorf("%s: export.data is not valid 1PUX JSON", path)
	}

	out := map[string]string{}
	for _, acct := range doc.Accounts {
		for _, vlt := range acct.Vaults {
			for _, item := range vlt.Items {
				collectPUXItem(out, item)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no credentials found in %s", path)
	}
	return out, nil
}

func collectPUXItem(out map[string]string, item puxItem) {
	if item.State == "archived" {
		return
	}
	title := item.Overview.Title
	if title == "" {
		return
	}
	for _, f := range item.Details.LoginFields {
		if f.Designation == "password" && f.Value != "" {
			addCredential(out, f.Value, title)
		}
	}
	for _, sec := range item.Details.Sections {
		for _, f := range sec.Fields {
			val := concealedValue(f.Value)
			if val == "" {
				continue
			}
			label := f.Title
			if label == "" {
				label = f.ID
			}
			addCredential(out, val, title, label)
		}
	}
}

// concealedValue returns a section field's value when it is a concealed one.
// A field's value is either a bare string or a single-key object naming its
// type; only "concealed" holds a secret, so every other type is skipped rather
// than imported as though it were one.
func concealedValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var typed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &typed); err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(typed["concealed"], &s); err != nil {
		return ""
	}
	return s
}

// addCredential stores val under a name derived from parts, keeping both
// values when two items produce the same name rather than silently dropping
// one. A collision suffix is preferable to a lost secret.
func addCredential(out map[string]string, val string, parts ...string) {
	base := CredentialName(parts...)
	if base == "" {
		return
	}
	name := base
	for n := 2; ; n++ {
		existing, taken := out[name]
		if !taken {
			break
		}
		if existing == val {
			return // the same secret under the same name; nothing to add
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
	out[name] = val
}

// CredentialName turns an item title (and optional field label) into an
// environment-variable-shaped name: upper snake case, with runs of other
// characters collapsed to one underscore. The result still has to pass
// ValidateEnvName, which the import path applies before anything is written.
func CredentialName(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		for _, r := range p {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune(r - 32)
			case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
				b.WriteRune(r)
			default:
				b.WriteByte('_')
			}
		}
		b.WriteByte('_')
	}
	name := strings.Trim(collapseUnderscores(b.String()), "_")
	if name == "" {
		return ""
	}
	// A leading digit is not a valid identifier.
	if name[0] >= '0' && name[0] <= '9' {
		name = "X_" + name
	}
	return name
}

func collapseUnderscores(s string) string {
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if r == '_' && prev == '_' {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// SortedNames returns a pair map's keys in order.
func SortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
