// Package store abstracts TOTP secret storage so the CLI can be unit-tested
// without touching the real macOS Keychain.
package store

import "errors"

// Marker is the value placed in the generic-password Description field to
// flag entries managed by this tool. Kept identical to the original zsh
// plugin so existing entries remain compatible.
const Marker = "TOTP (totp.plugin.zsh)"

// ErrNotFound is returned when a Keychain item cannot be located.
var ErrNotFound = errors.New("entry not found")

// Store is the storage interface for TOTP secrets. The production
// implementation is backed by the macOS Keychain; tests use an in-memory
// mock.
type Store interface {
	// Add stores or updates a secret under the given name and applies the
	// management marker.
	Add(name, secret string) error
	// Get returns the base32 secret for name, or ErrNotFound.
	Get(name string) (string, error)
	// Remove deletes the entry for name. Returns ErrNotFound if it does not
	// exist.
	Remove(name string) error
	// List returns service names. When markedOnly is true, only entries
	// carrying Marker are returned. pattern is a substring filter; empty
	// pattern returns everything.
	List(markedOnly bool, pattern string) ([]string, error)
	// Tag applies the management marker to an existing entry. Idempotent —
	// calling it on an already-marked entry is a no-op success.
	Tag(name string) error
}
