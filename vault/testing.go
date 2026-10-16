package vault

import "testing"

// testScryptWorkFactor, when non-zero, replaces age's default scrypt work
// factor (2^18) for files the age backend writes. Only tests set it.
var testScryptWorkFactor int

// UseFastScryptForTests makes the age backend encrypt with a trivially cheap
// scrypt work factor, so suites that open many temporary vaults run in seconds
// rather than minutes. It panics outside `go test`: a production binary must
// never write a vault that is cheap to brute-force.
func UseFastScryptForTests() {
	if !testing.Testing() {
		panic("vault.UseFastScryptForTests called outside of go test")
	}
	testScryptWorkFactor = 10
}
