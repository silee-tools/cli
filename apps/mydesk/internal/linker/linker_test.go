package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkAndUnlink(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source file
	srcFile := filepath.Join(srcDir, "test.conf")
	if err := os.WriteFile(srcFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	dstFile := filepath.Join(dstDir, "test.conf")

	entries := []LinkEntry{{
		Source: "test.conf",
		Target: dstFile,
		SrcAbs: srcFile,
		DstAbs: dstFile,
	}}

	// Link
	l := New(entries, false)
	r := l.Link()
	if r.Failed != 0 {
		t.Fatalf("link failed: %v", r.Errors)
	}
	if r.Linked != 1 {
		t.Errorf("linked = %d, want 1", r.Linked)
	}

	// Verify symlink
	target, err := os.Readlink(dstFile)
	if err != nil {
		t.Fatal("target is not a symlink")
	}
	if target != srcFile {
		t.Errorf("symlink points to %q, want %q", target, srcFile)
	}

	// Link again should skip
	r = l.Link()
	if r.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", r.Skipped)
	}

	// Unlink
	r = l.Unlink()
	if r.Failed != 0 {
		t.Fatalf("unlink failed: %v", r.Errors)
	}
	if _, err := os.Lstat(dstFile); !os.IsNotExist(err) {
		t.Error("target should be removed after unlink")
	}
}

func TestLinkWithBackup(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.conf")
	os.WriteFile(srcFile, []byte("new content"), 0644)

	// Create existing file at target
	dstFile := filepath.Join(dstDir, "test.conf")
	os.WriteFile(dstFile, []byte("old content"), 0644)

	entries := []LinkEntry{{
		Source: "test.conf",
		Target: dstFile,
		SrcAbs: srcFile,
		DstAbs: dstFile,
	}}

	l := New(entries, false)
	r := l.Link()
	if r.BackedUp != 1 {
		t.Errorf("backedUp = %d, want 1", r.BackedUp)
	}

	// Verify backup exists
	backupFile := dstFile + BackupSuffix
	data, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatal("backup file should exist")
	}
	if string(data) != "old content" {
		t.Errorf("backup content = %q, want 'old content'", data)
	}

	// Unlink should restore backup
	r = l.Unlink()
	data, err = os.ReadFile(dstFile)
	if err != nil {
		t.Fatal("original should be restored")
	}
	if string(data) != "old content" {
		t.Errorf("restored content = %q, want 'old content'", data)
	}
}

func TestLinkDryRun(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.conf")
	os.WriteFile(srcFile, []byte("content"), 0644)

	dstFile := filepath.Join(dstDir, "test.conf")

	entries := []LinkEntry{{
		Source: "test.conf",
		Target: dstFile,
		SrcAbs: srcFile,
		DstAbs: dstFile,
	}}

	l := New(entries, true) // dry-run
	r := l.Link()
	if r.Linked != 1 {
		t.Errorf("linked = %d, want 1", r.Linked)
	}

	// File should NOT be created
	if _, err := os.Lstat(dstFile); !os.IsNotExist(err) {
		t.Error("dry-run should not create symlink")
	}
}

func TestLinkSourceNotFound(t *testing.T) {
	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "test.conf")

	entries := []LinkEntry{{
		Source: "test.conf",
		Target: dstFile,
		SrcAbs: "/nonexistent/source",
		DstAbs: dstFile,
	}}

	l := New(entries, false)
	r := l.Link()
	if r.Failed != 1 {
		t.Errorf("failed = %d, want 1", r.Failed)
	}
}
