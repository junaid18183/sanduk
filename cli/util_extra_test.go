package cli

import "testing"

func TestFilterExcept(t *testing.T) {
	env := map[string]string{"A": "1", "B": "2", "C": "3"}

	got := filterExcept(env, []string{"B"})
	if len(got) != 2 || got["A"] != "1" || got["C"] != "3" {
		t.Fatalf("filterExcept = %v, want A and C", got)
	}

	// Unlike --only, an unknown exclusion is a harmless no-op.
	got = filterExcept(env, []string{"NOPE"})
	if len(got) != 3 {
		t.Fatalf("filterExcept with an unknown key = %v, want all keys", got)
	}

	got = filterExcept(env, nil)
	if len(got) != 3 {
		t.Fatalf("filterExcept(nil) = %v, want all keys", got)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"A", "A", "B", "A", "B"})
	want := []string{"A", "B"}
	if len(got) != len(want) {
		t.Fatalf("dedupeStrings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupeStrings = %v, want %v", got, want)
		}
	}
}

func TestSortedMapKeys(t *testing.T) {
	got := sortedMapKeys(map[string]string{"C": "3", "A": "1", "B": "2"})
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("sortedMapKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedMapKeys = %v, want %v", got, want)
		}
	}
}

func TestOverlap(t *testing.T) {
	got := overlap([]string{"A", "C", "B"}, []string{"B", "C", "D"})
	want := []string{"B", "C"}
	if len(got) != len(want) {
		t.Fatalf("overlap = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("overlap = %v, want %v", got, want)
		}
	}
	if got := overlap([]string{"A"}, []string{"B"}); len(got) != 0 {
		t.Fatalf("overlap with no common keys = %v, want empty", got)
	}
}
