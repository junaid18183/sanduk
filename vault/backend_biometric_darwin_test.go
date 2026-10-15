//go:build darwin && cgo

package vault

import "testing"

// This test only inspects the constructed keyring; it never talks to the
// keychain, so it does not trigger a user-presence prompt.
func TestBiometricKeyringUsesSeparateNamespace(t *testing.T) {
	kr, err := newBiometricKeyring("sanduk")
	if err != nil {
		t.Fatalf("newBiometricKeyring: %v", err)
	}
	bk, ok := kr.(*biometricKeyring)
	if !ok {
		t.Fatalf("unexpected keyring type %T", kr)
	}
	if bk.service != "sanduk-biometric" {
		t.Fatalf("service = %q, want sanduk-biometric", bk.service)
	}
}
