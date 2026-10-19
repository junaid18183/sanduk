package vault

import (
	"strings"
	"testing"
)

func TestScannerFindsAValue(t *testing.T) {
	sc := NewScanner(map[string]string{"stripe": "sk-test-abcdefgh"}, 8)
	src := "one\nkey = \"sk-test-abcdefgh\"\nthree\n"

	got, ok, err := sc.ScanReader("app.go", strings.NewReader(src))
	if err != nil || !ok {
		t.Fatalf("scan: ok=%v err=%v", ok, err)
	}
	if len(got) != 1 {
		t.Fatalf("matches = %v", got)
	}
	if got[0].Entry != "stripe" || got[0].Where != "app.go" || got[0].Line != 2 {
		t.Fatalf("match = %+v", got[0])
	}
}

func TestScannerSkipsShortValues(t *testing.T) {
	sc := NewScanner(map[string]string{"tiny": "abc", "long": "sk-test-abcdefgh"}, 8)
	if sc.Needles() != 1 {
		t.Fatalf("needles = %d, want 1", sc.Needles())
	}
	if len(sc.Skipped) != 1 || sc.Skipped[0] != "tiny" {
		t.Fatalf("skipped = %v", sc.Skipped)
	}
	// The short value must not match, or every line would be a finding.
	got, _, err := sc.ScanReader("x", strings.NewReader("abc abc abc\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("short value matched: %v", got)
	}
}

// Two entries sharing a value are both reported: either could be the leak.
func TestScannerReportsEveryEntrySharingAValue(t *testing.T) {
	sc := NewScanner(map[string]string{"a": "sk-test-shared", "b": "sk-test-shared"}, 8)
	got, _, err := sc.ScanReader("x", strings.NewReader("sk-test-shared\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("matches = %v, want both entries", got)
	}
}

func TestScannerStopsAtBinary(t *testing.T) {
	sc := NewScanner(map[string]string{"k": "sk-test-abcdefgh"}, 8)
	src := "text\n\x00binary\nsk-test-abcdefgh\n"

	got, ok, err := sc.ScanReader("blob.bin", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("binary content was reported as searched")
	}
	if len(got) != 0 {
		t.Fatalf("matched past the NUL: %v", got)
	}
}

func TestScannerHandlesAVeryLongLine(t *testing.T) {
	sc := NewScanner(map[string]string{"k": "sk-test-abcdefgh"}, 8)
	long := strings.Repeat("x", maxScanLine+10)

	_, ok, err := sc.ScanReader("bundle.js", strings.NewReader(long))
	if err != nil {
		t.Fatalf("a long line should not be an error: %v", err)
	}
	if ok {
		t.Fatal("an unsearchable file was reported as searched")
	}
}

// A match must never carry the value itself: the report is meant to be safe
// to paste.
func TestMatchCarriesNoValue(t *testing.T) {
	const secret = "sk-test-abcdefgh"
	sc := NewScanner(map[string]string{"stripe": secret}, 8)
	got, _, err := sc.ScanReader("app.go", strings.NewReader("key="+secret+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if strings.Contains(m.Entry+m.Where+m.Note, secret) {
			t.Fatalf("match leaked the value: %+v", m)
		}
	}
}

func TestScanValuesCoversEveryKind(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("flat", "sk-test-flat-value"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddGroup("warden", map[string]string{"DB_URL": "postgres://secret"}); err != nil {
		t.Fatal(err)
	}
	if err := v.AddFile("gcp", []byte(`{"private_key":"sk-test-file"}`), "sa.json"); err != nil {
		t.Fatal(err)
	}
	if err := v.LinkGroupField("warden", "FLAT_KEY", "flat"); err != nil {
		t.Fatal(err)
	}

	got, err := v.ScanValues()
	if err != nil {
		t.Fatal(err)
	}
	if got["flat"] != "sk-test-flat-value" {
		t.Fatalf("flat secret missing: %v", SortedNames(got))
	}
	if got["warden.DB_URL"] != "postgres://secret" {
		t.Fatalf("group field missing: %v", SortedNames(got))
	}
	if !strings.Contains(got["gcp"], "sk-test-file") {
		t.Fatalf("file secret missing: %v", SortedNames(got))
	}
	// A link is not a value of its own; the target is already in the set.
	if _, ok := got["warden.FLAT_KEY"]; ok {
		t.Fatalf("a link was scanned as its own value: %v", SortedNames(got))
	}
}
