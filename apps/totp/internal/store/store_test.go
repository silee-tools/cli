package store

import (
	"errors"
	"reflect"
	"testing"
)

// Compile-time check: MockStore satisfies the Store interface.
var _ Store = (*MockStore)(nil)

func TestMock_AddGetRemoveCycle(t *testing.T) {
	m := NewMock()

	if err := m.Add("acme", "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := m.Get("acme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("Get returned %q", got)
	}
	if err := m.Remove("acme"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := m.Get("acme"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after Remove, got %v", err)
	}
	if err := m.Remove("acme"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound on second Remove, got %v", err)
	}
}

func TestMock_ListMarkedAndAll(t *testing.T) {
	m := NewMock()
	_ = m.Add("MS: alice@example.com", "AAAA")
	_ = m.Add("GH: alice", "BBBB")
	m.AddUnmarked("legacy: bob", "CCCC")

	all, err := m.List(false, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	wantAll := []string{"GH: alice", "MS: alice@example.com", "legacy: bob"}
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("List all = %v, want %v", all, wantAll)
	}

	marked, err := m.List(true, "")
	if err != nil {
		t.Fatalf("List marked: %v", err)
	}
	wantMarked := []string{"GH: alice", "MS: alice@example.com"}
	if !reflect.DeepEqual(marked, wantMarked) {
		t.Errorf("List marked = %v, want %v", marked, wantMarked)
	}

	filtered, err := m.List(true, "MS:")
	if err != nil {
		t.Fatalf("List pattern: %v", err)
	}
	wantFiltered := []string{"MS: alice@example.com"}
	if !reflect.DeepEqual(filtered, wantFiltered) {
		t.Errorf("List pattern = %v, want %v", filtered, wantFiltered)
	}
}

func TestMock_TagIdempotent(t *testing.T) {
	m := NewMock()
	m.AddUnmarked("legacy", "ZZZZ")

	// Before tag: not in marked list.
	marked, _ := m.List(true, "")
	if len(marked) != 0 {
		t.Fatalf("expected no marked entries, got %v", marked)
	}

	if err := m.Tag("legacy"); err != nil {
		t.Fatalf("Tag: %v", err)
	}
	marked, _ = m.List(true, "")
	if len(marked) != 1 || marked[0] != "legacy" {
		t.Fatalf("after Tag, marked = %v", marked)
	}

	// Calling again is a no-op success.
	if err := m.Tag("legacy"); err != nil {
		t.Fatalf("second Tag: %v", err)
	}

	if err := m.Tag("nonexistent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Tag missing entry = %v, want ErrNotFound", err)
	}
}
