package vault

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Without the lock, concurrent load-modify-save cycles drop each other's
// updates. flock is per open file, so goroutines exercise it like processes.
func TestUpdateConfigLosesNoUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- UpdateConfig(path, func(c *Config) error {
				c.RecordSecret(fmt.Sprintf("s%02d", i), "", "")
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Secrets) != n {
		t.Fatalf("%d of %d concurrent updates survived", len(c.Secrets), n)
	}
}

func TestMetadataDrift(t *testing.T) {
	c := &Config{
		Secrets: map[string]SecretMeta{"openai": {}, "gone": {}},
		Groups:  map[string]GroupMeta{"warden": {}},
	}
	untracked, orphaned := MetadataDrift([]string{"openai", "warden", "new"}, c)
	if len(untracked) != 1 || untracked[0] != "new" {
		t.Fatalf("untracked = %v, want [new]", untracked)
	}
	if len(orphaned) != 1 || orphaned[0] != "gone" {
		t.Fatalf("orphaned = %v, want [gone]", orphaned)
	}
}
