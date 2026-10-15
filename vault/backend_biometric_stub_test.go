//go:build !darwin || !cgo

package vault

import "testing"

func TestBiometricKeyringUnavailable(t *testing.T) {
	if _, err := newBiometricKeyring("sanduk"); err == nil {
		t.Fatal("expected an error when the biometric backend is unavailable")
	}
}
