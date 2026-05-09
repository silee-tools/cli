package store

import (
	"sort"
	"strings"
	"sync"
)

// MockStore is an in-memory Store for testing.
type MockStore struct {
	mu      sync.Mutex
	entries map[string]mockEntry
}

type mockEntry struct {
	secret string
	marked bool
}

// NewMock returns an empty MockStore.
func NewMock() *MockStore {
	return &MockStore{entries: map[string]mockEntry{}}
}

func (m *MockStore) Add(name, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[name] = mockEntry{secret: secret, marked: true}
	return nil
}

func (m *MockStore) Get(name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[name]
	if !ok {
		return "", ErrNotFound
	}
	return e.secret, nil
}

func (m *MockStore) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[name]; !ok {
		return ErrNotFound
	}
	delete(m.entries, name)
	return nil
}

func (m *MockStore) List(markedOnly bool, pattern string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for name, e := range m.entries {
		if markedOnly && !e.marked {
			continue
		}
		if pattern != "" && !strings.Contains(name, pattern) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (m *MockStore) Tag(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[name]
	if !ok {
		return ErrNotFound
	}
	e.marked = true
	m.entries[name] = e
	return nil
}

// AddUnmarked is a test helper to seed pre-existing keychain entries that do
// not yet carry the management marker.
func (m *MockStore) AddUnmarked(name, secret string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[name] = mockEntry{secret: secret, marked: false}
}
