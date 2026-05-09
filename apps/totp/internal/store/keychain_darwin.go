//go:build darwin

package store

import (
	"fmt"
	"os/user"
	"sort"
	"strings"

	"github.com/keybase/go-keychain"
)

// Keychain is the macOS Keychain-backed Store implementation.
type Keychain struct {
	account string
}

// NewKeychain returns a Store that reads/writes generic-password items in the
// user's default Keychain. account defaults to the current $USER.
func NewKeychain() (*Keychain, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve current user: %w", err)
	}
	return &Keychain{account: u.Username}, nil
}

func (k *Keychain) baseItem(name string) keychain.Item {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(name)
	item.SetAccount(k.account)
	return item
}

func (k *Keychain) Add(name, secret string) error {
	// Match `security add-generic-password -U` semantics: replace existing
	// entry idempotently.
	_ = k.deleteRaw(name)
	item := k.baseItem(name)
	item.SetLabel(name)
	item.SetDescription(Marker)
	item.SetData([]byte(secret))
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)
	if err := keychain.AddItem(item); err != nil {
		return fmt.Errorf("keychain add: %w", err)
	}
	return nil
}

func (k *Keychain) Get(name string) (string, error) {
	query := k.baseItem(name)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return "", fmt.Errorf("keychain query: %w", err)
	}
	if len(results) == 0 {
		return "", ErrNotFound
	}
	return string(results[0].Data), nil
}

func (k *Keychain) deleteRaw(name string) error {
	return keychain.DeleteItem(k.baseItem(name))
}

func (k *Keychain) Remove(name string) error {
	if err := k.deleteRaw(name); err != nil {
		if err == keychain.ErrorItemNotFound {
			return ErrNotFound
		}
		return fmt.Errorf("keychain delete: %w", err)
	}
	return nil
}

// List enumerates generic-password items for the current account. When
// markedOnly is true, only items whose Description equals Marker are
// returned.
func (k *Keychain) List(markedOnly bool, pattern string) ([]string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetAccount(k.account)
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, fmt.Errorf("keychain list: %w", err)
	}
	seen := map[string]bool{}
	for _, r := range results {
		if r.Service == "" {
			continue
		}
		if markedOnly && r.Description != Marker {
			continue
		}
		if pattern != "" && !strings.Contains(r.Service, pattern) {
			continue
		}
		seen[r.Service] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

// Tag applies the management marker to an existing entry by re-storing the
// secret with Description=Marker. Idempotent.
func (k *Keychain) Tag(name string) error {
	secret, err := k.Get(name)
	if err != nil {
		return err
	}
	return k.Add(name, secret)
}
