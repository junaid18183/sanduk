package cli

import "testing"

func TestDiffKeys(t *testing.T) {
	present := map[string]string{"A": "1", "B": "2", "D": "4"}
	template := map[string]string{"A": "x", "B": "y", "C": "z"}

	missing, extra := diffKeys(present, template)

	if len(missing) != 1 || missing[0] != "C" {
		t.Fatalf("missing = %v, want [C]", missing)
	}
	if len(extra) != 1 || extra[0] != "D" {
		t.Fatalf("extra = %v, want [D]", extra)
	}
}

func TestDiffKeysIdentical(t *testing.T) {
	same := map[string]string{"A": "1", "B": "2"}

	missing, extra := diffKeys(same, map[string]string{"A": "x", "B": "y"})
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("missing = %v, extra = %v; want none", missing, extra)
	}
}

func TestDiffKeysEmptyTemplate(t *testing.T) {
	missing, extra := diffKeys(map[string]string{"A": "1"}, map[string]string{})
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
	if len(extra) != 1 || extra[0] != "A" {
		t.Fatalf("extra = %v, want [A]", extra)
	}
}

func TestSortedKeysIsSorted(t *testing.T) {
	got := sortedMapKeys(map[string]string{"C": "", "A": "", "B": ""})
	want := []string{"A", "B", "C"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedKeys = %v, want %v", got, want)
		}
	}
}
