//go:build darwin && cgo

package vault

import (
	"testing"

	"github.com/99designs/keyring"
)

// The macOS Keychain backend in 99designs/keyring is compiled only under
// `darwin && cgo`. If it is missing, a macOS build has no OS keystore and
// silently degrades to the encrypted file. This test fails loudly instead.
func TestNativeBackendsIncludesKeychainOnMacOS(t *testing.T) {
	for _, b := range nativeBackends() {
		if b == keyring.KeychainBackend {
			return
		}
	}
	t.Fatalf("keychain backend is not compiled in (got %v); build macOS binaries with CGO_ENABLED=1",
		nativeBackends())
}
