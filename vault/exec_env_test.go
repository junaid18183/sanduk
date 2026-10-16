package vault

import (
	"strings"
	"testing"
)

func TestBuildEnvScrubsSandukVars(t *testing.T) {
	t.Setenv("SANDUK_FILE_PASSPHRASE", "[REDACTED]")
	t.Setenv("SANDUK_KEYCHAIN_PASSPHRASE", "[REDACTED]")
	t.Setenv("SANDUK_BACKEND", "file")
	t.Setenv("KEEP_ME", "yes")

	out := buildEnv(map[string]string{"INJECTED": "value"})

	got := map[string]string{}
	for _, kv := range out {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}

	for _, k := range []string{"SANDUK_FILE_PASSPHRASE", "SANDUK_KEYCHAIN_PASSPHRASE", "SANDUK_BACKEND"} {
		if _, ok := got[k]; ok {
			t.Fatalf("%s leaked into the child environment", k)
		}
	}
	if got["KEEP_ME"] != "yes" {
		t.Fatal("unrelated variables must be preserved")
	}
	if got["INJECTED"] != "value" {
		t.Fatal("override missing from the child environment")
	}
}

func TestBuildEnvStableOrder(t *testing.T) {
	a := buildEnv(map[string]string{"B": "2", "A": "1", "C": "3"})
	b := buildEnv(map[string]string{"C": "3", "A": "1", "B": "2"})
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Fatalf("override order is not deterministic:\n%v\n%v", a, b)
	}
}
