package vault

import "testing"

func TestValidEnvName(t *testing.T) {
	good := []string{"A", "_x", "AWS_ACCESS_KEY_ID", "a1", "X_AUTH_TOKEN_URDU"}
	for _, n := range good {
		if !ValidEnvName(n) {
			t.Errorf("ValidEnvName(%q) = false, want true", n)
		}
	}
	bad := []string{"", "1A", "A B", "A-B", "A.B", "A=B", "A\tB"}
	for _, n := range bad {
		if ValidEnvName(n) {
			t.Errorf("ValidEnvName(%q) = true, want false", n)
		}
	}
}

func TestValidateEnvNameRejectsSensitive(t *testing.T) {
	for _, n := range []string{"LD_PRELOAD", "NODE_OPTIONS", "dyld_insert_libraries"} {
		if err := ValidateEnvName(n); err == nil {
			t.Errorf("ValidateEnvName(%q) = nil, want an error", n)
		}
	}
	if err := ValidateEnvName("DB_URL"); err != nil {
		t.Fatalf("ValidateEnvName(DB_URL) = %v, want nil", err)
	}
}

func TestAddGroupRejectsInvalidFieldName(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddGroup("g", map[string]string{"A B": "1"}); err == nil {
		t.Fatal("AddGroup accepted a field name with a space")
	}
	if err := v.MergeGroup("g", map[string]string{"A B": "1"}); err == nil {
		t.Fatal("MergeGroup accepted a field name with a space")
	}
}
