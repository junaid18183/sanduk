package vault

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectImportFormat(t *testing.T) {
	for path, want := range map[string]string{
		"creds.csv":          ImportCSV,
		"CREDS.CSV":          ImportCSV,
		"ABC123.1pux":        Import1PUX,
		".env":               ImportDotenv,
		".env.production":    ImportDotenv,
		"/tmp/anything.else": ImportDotenv,
	} {
		if got := DetectImportFormat(path); got != want {
			t.Fatalf("%s -> %s, want %s", path, got, want)
		}
	}
}

func TestCredentialName(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"Stripe"}, "STRIPE"},
		{[]string{"My API key"}, "MY_API_KEY"},
		{[]string{"Dropbox", "PIN"}, "DROPBOX_PIN"},
		{[]string{"api.example.com"}, "API_EXAMPLE_COM"},
		{[]string{"  spaced  out  "}, "SPACED_OUT"},
		{[]string{"2fa backup"}, "X_2FA_BACKUP"},
		{[]string{"---"}, ""},
		{[]string{""}, ""},
	} {
		if got := CredentialName(tc.in...); got != tc.want {
			t.Fatalf("CredentialName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseCSVPrefersThePasswordColumn(t *testing.T) {
	csv := "folder,name,login_username,login_password,value\n" +
		"work,Stripe,me@example.com,sk-test-one,ignored\n"
	got, err := ParseCSVExport(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if got["STRIPE"] != "sk-test-one" {
		t.Fatalf("got %v", got)
	}
}

func TestParseCSVReadsCustomFields(t *testing.T) {
	csv := "name,login_password,fields\n" +
		"\"Acme API\",,\"API Key: sk-test-two\nWebhook Secret: whsec-test\"\n"
	got, err := ParseCSVExport(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if got["ACME_API_API_KEY"] != "sk-test-two" {
		t.Fatalf("API key missing: %v", keysOf(got))
	}
	if got["ACME_API_WEBHOOK_SECRET"] != "whsec-test" {
		t.Fatalf("webhook secret missing: %v", keysOf(got))
	}
}

func TestParseCSVTitleColumnAlias(t *testing.T) {
	csv := "Title,Username,Password,Notes\nOpenAI,me,sk-test-three,\n"
	got, err := ParseCSVExport(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if got["OPENAI"] != "sk-test-three" {
		t.Fatalf("got %v", keysOf(got))
	}
}

func TestParseCSVErrors(t *testing.T) {
	for name, in := range map[string]string{
		"header only":    "name,password\n",
		"no name column": "colour,password\nred,sk-test\n",
		"no value":       "name,username\nStripe,me\n",
		"empty values":   "name,password\nStripe,\n",
	} {
		if _, err := ParseCSVExport(strings.NewReader(in)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

// Two items with the same title must both survive, under distinct names.
func TestCSVCollisionKeepsBoth(t *testing.T) {
	csv := "name,password\nStripe,sk-test-one\nStripe,sk-test-two\n"
	got, err := ParseCSVExport(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if got["STRIPE"] != "sk-test-one" || got["STRIPE_2"] != "sk-test-two" {
		t.Fatalf("got %v", got)
	}
}

// The same secret under the same name is one entry, not two.
func TestCSVExactDuplicateIsCollapsed(t *testing.T) {
	csv := "name,password\nStripe,sk-test-one\nStripe,sk-test-one\n"
	got, err := ParseCSVExport(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

// write1PUX builds a minimal archive matching the documented structure.
func write1PUX(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.1pux")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("export.data")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

const puxSample = `{"accounts":[{"vaults":[{"items":[
 {"state":"active","categoryUuid":"001",
  "overview":{"title":"Dropbox"},
  "details":{
    "loginFields":[
      {"value":"me@example.com","designation":"username"},
      {"value":"sk-test-login","designation":"password"}],
    "notesPlain":"a note",
    "sections":[{"title":"Security","fields":[
      {"title":"PIN","value":{"concealed":"sk-test-pin"}},
      {"title":"Website","value":{"string":"https://example.com"}}]}]}},
 {"state":"archived","overview":{"title":"Old"},
  "details":{"loginFields":[{"value":"sk-test-archived","designation":"password"}]}}
]}]}]}`

func TestParse1PUXTakesOnlySecrets(t *testing.T) {
	got, err := Parse1PUX(write1PUX(t, puxSample))
	if err != nil {
		t.Fatal(err)
	}
	if got["DROPBOX"] != "sk-test-login" {
		t.Fatalf("login password missing: %v", keysOf(got))
	}
	if got["DROPBOX_PIN"] != "sk-test-pin" {
		t.Fatalf("concealed field missing: %v", keysOf(got))
	}
	for name, val := range got {
		if strings.Contains(val, "@example.com") || strings.Contains(val, "https://") {
			t.Fatalf("%s carried a non-secret field: the importer must skip it", name)
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(got), keysOf(got))
	}
}

func TestParse1PUXSkipsArchivedItems(t *testing.T) {
	got, err := Parse1PUX(write1PUX(t, puxSample))
	if err != nil {
		t.Fatal(err)
	}
	for _, val := range got {
		if val == "sk-test-archived" {
			t.Fatal("an archived item was imported")
		}
	}
}

func TestParse1PUXErrors(t *testing.T) {
	if _, err := Parse1PUX(write1PUX(t, "not json")); err == nil {
		t.Fatal("expected a parse error")
	}
	if _, err := Parse1PUX(write1PUX(t, `{"accounts":[]}`)); err == nil {
		t.Fatal("expected an empty-archive error")
	}
	if _, err := Parse1PUX(filepath.Join(t.TempDir(), "missing.1pux")); err == nil {
		t.Fatal("expected a missing-file error")
	}
	// A zip with no export.data is not a 1PUX archive.
	path := filepath.Join(t.TempDir(), "other.1pux")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	_, _ = zw.Create("something.txt")
	_ = zw.Close()
	_ = f.Close()
	if _, err := Parse1PUX(path); err == nil || !strings.Contains(err.Error(), "export.data") {
		t.Fatalf("err = %v, want a complaint about export.data", err)
	}
}

// A parse failure must not quote the file, which is full of secrets.
func TestParse1PUXErrorDoesNotQuoteTheFile(t *testing.T) {
	_, err := Parse1PUX(write1PUX(t, `{"accounts": sk-test-leaked}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "sk-test-leaked") {
		t.Fatalf("the error quoted the file contents: %v", err)
	}
}

func keysOf(m map[string]string) []string { return SortedNames(m) }
