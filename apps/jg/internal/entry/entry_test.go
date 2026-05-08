package entry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestFile(t *testing.T) string {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), ".jg")
	DataFile = tmp
	return tmp
}

func TestParseLineValid(t *testing.T) {
	e, ok := parseLine("/Users/test/repo|5|1700000000")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.Path != "/Users/test/repo" || e.Rank != 5 || e.Timestamp != 1700000000 {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestParseLinePathWithPipes(t *testing.T) {
	e, ok := parseLine("/Users/test/repo|with|pipes|5|1700000000")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.Path != "/Users/test/repo|with|pipes" || e.Rank != 5 || e.Timestamp != 1700000000 {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestParseLineInvalid(t *testing.T) {
	tests := []string{
		"",
		"no-pipes",
		"path|notanumber|123",
		"path|1|notanumber",
		"too|many|pipes|here",
	}
	for _, line := range tests {
		if _, ok := parseLine(line); ok {
			t.Errorf("expected parseLine(%q) to fail", line)
		}
	}
}

func TestFormatLineRoundTrip(t *testing.T) {
	original := Entry{Path: "/test/repo", Rank: 3, Timestamp: 1700000000}
	line := formatLine(original)
	parsed, ok := parseLine(line)
	if !ok {
		t.Fatalf("failed to parse formatted line: %s", line)
	}
	if parsed.Path != original.Path || parsed.Rank != original.Rank || parsed.Timestamp != original.Timestamp {
		t.Fatalf("round-trip failed: got %+v, want %+v", parsed, original)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	setupTestFile(t)
	entries, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(entries))
	}
}

func TestSaveAndLoad(t *testing.T) {
	setupTestFile(t)

	want := []Entry{
		{Path: "/repo/a", Rank: 5, Timestamp: 1700000000},
		{Path: "/repo/b", Rank: 3, Timestamp: 1700000100},
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Path != want[i].Path || got[i].Rank != want[i].Rank || got[i].Timestamp != want[i].Timestamp {
			t.Errorf("entry[%d] mismatch: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAddOrUpdate(t *testing.T) {
	setupTestFile(t)

	// First add
	if err := AddOrUpdate("/repo/test"); err != nil {
		t.Fatalf("AddOrUpdate: %v", err)
	}

	entries, _ := Load()
	if len(entries) != 1 || entries[0].Rank != 1 {
		t.Fatalf("expected 1 entry with rank 1, got %+v", entries)
	}

	// Second add (update)
	if err := AddOrUpdate("/repo/test"); err != nil {
		t.Fatalf("AddOrUpdate: %v", err)
	}

	entries, _ = Load()
	if len(entries) != 1 || entries[0].Rank != 2 {
		t.Fatalf("expected 1 entry with rank 2, got %+v", entries)
	}
}

func TestRemove(t *testing.T) {
	setupTestFile(t)

	Save([]Entry{
		{Path: "/repo/a", Rank: 1, Timestamp: 1700000000},
		{Path: "/repo/b", Rank: 1, Timestamp: 1700000000},
	})

	removed, err := Remove("/repo/a")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("expected removed to be true")
	}

	entries, _ := Load()
	if len(entries) != 1 || entries[0].Path != "/repo/b" {
		t.Fatalf("unexpected entries after remove: %+v", entries)
	}
}

func TestClean(t *testing.T) {
	setupTestFile(t)

	existingDir := t.TempDir()
	Save([]Entry{
		{Path: existingDir, Rank: 1, Timestamp: time.Now().Unix()},
		{Path: "/nonexistent/path/repo", Rank: 1, Timestamp: 1700000000},
	})

	removed, err := Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	entries, _ := Load()
	if len(entries) != 1 || entries[0].Path != existingDir {
		t.Fatalf("unexpected entries after clean: %+v", entries)
	}
}

func TestLoadSkipsMalformedLines(t *testing.T) {
	tmp := setupTestFile(t)
	content := "/valid/path|3|1700000000\nbad line\n/another/valid|1|1700000100\n"
	os.WriteFile(tmp, []byte(content), 0644)

	entries, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}
