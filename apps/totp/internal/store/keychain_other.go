//go:build !darwin

package store

import "errors"

// Keychain is a stub on non-darwin platforms. The CLI is macOS-only; this
// stub exists solely so tests and `go vet` can run on Linux CI.
type Keychain struct{}

// NewKeychain returns an error on non-darwin platforms.
func NewKeychain() (*Keychain, error) {
	return nil, errors.New("totp: macOS Keychain is required (this build does not support the current OS)")
}

func (*Keychain) Add(string, string) error            { return errors.New("unsupported") }
func (*Keychain) Get(string) (string, error)          { return "", errors.New("unsupported") }
func (*Keychain) Remove(string) error                 { return errors.New("unsupported") }
func (*Keychain) List(bool, string) ([]string, error) { return nil, errors.New("unsupported") }
func (*Keychain) Tag(string) error                    { return errors.New("unsupported") }
