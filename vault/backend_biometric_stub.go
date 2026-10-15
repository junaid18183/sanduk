//go:build !darwin || !cgo

package vault

import (
	"errors"

	"github.com/99designs/keyring"
)

// newBiometricKeyring is unavailable off macOS or without cgo.
func newBiometricKeyring(service string) (keyring.Keyring, error) {
	return nil, errors.New("the keychain-biometric backend requires macOS with cgo enabled")
}
