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

func TestParseLineWithPipeInPath(t *testing.T) {
	// path 안에 '|' 가 있어도 LastIndex 기반 파서가 올바르게 동작해야 한다.
	got, ok := parseLine("/repo/wt|with|pipes|1.5|1700000000")
	if !ok {
		t.Fatal("parseLine returned !ok")
	}
	if got.Path != "/repo/wt|with|pipes" {
		t.Errorf("Path = %q, want %q", got.Path, "/repo/wt|with|pipes")
	}
	if got.Rank != 1.5 {
		t.Errorf("Rank = %v, want 1.5", got.Rank)
	}
	if got.Timestamp != 1700000000 {
		t.Errorf("Timestamp = %v, want 1700000000", got.Timestamp)
	}
}
