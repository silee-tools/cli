package wtstore

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	DataFile = filepath.Join(dir, "worktrees")

	want := []Entry{
		{Path: "/repo/wt1", Rank: 2, Timestamp: 1700000000},
		{Path: "/repo/wt2", Rank: 5.5, Timestamp: 1700000100},
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "/repo/wt1" || got[1].Rank != 5.5 {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestAddOrUpdateBumpsRank(t *testing.T) {
	dir := t.TempDir()
	DataFile = filepath.Join(dir, "worktrees")

	if err := AddOrUpdate("/repo/wt1"); err != nil {
		t.Fatal(err)
	}
	if err := AddOrUpdate("/repo/wt1"); err != nil {
		t.Fatal(err)
	}
	entries, _ := Load()
	if len(entries) != 1 || entries[0].Rank != 2 {
		t.Errorf("AddOrUpdate did not bump rank: %+v", entries)
	}
}
