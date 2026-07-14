package jira

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteSnapshotCreatesPrivateFileAndAtomicallyReplacesIt(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "jira", "jira.example.test")
	path := filepath.Join(directory, "OMA-42.json")
	if err := WriteSnapshot(path, []byte(`{"version":1}`)); err != nil {
		t.Fatalf("first WriteSnapshot: %v", err)
	}
	assertFileContentAndMode(t, path, `{"version":1}`, 0o600)
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %o, want 700", got)
	}
	if err := WriteSnapshot(path, []byte(`{"version":2}`)); err != nil {
		t.Fatalf("replacement WriteSnapshot: %v", err)
	}
	assertFileContentAndMode(t, path, `{"version":2}`, 0o600)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory contains temporary artifacts: %#v", entries)
	}
}

func TestWriteSnapshotRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	path := filepath.Join(directory, "OMA-42.json")
	if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := WriteSnapshot(path, []byte("replacement")); err == nil {
		t.Fatal("WriteSnapshot replaced a symlink")
	}
	assertFileContentAndMode(t, target, "preserve", 0o600)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("snapshot symlink was changed: %v, %v", info, err)
	}
}

func TestWriteSnapshotFailurePreservesExistingCollision(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "OMA-42.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSnapshot(path, []byte("replacement")); err == nil {
		t.Fatal("WriteSnapshot replaced a directory collision")
	}
	assertFileContentAndMode(t, marker, "preserve", 0o600)
}

func assertFileContentAndMode(t *testing.T, path, want string, mode ...os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
	if len(mode) > 0 {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode[0] {
			t.Errorf("mode = %o, want %o", got, mode[0])
		}
	}
}
