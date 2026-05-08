package drift

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silee-tools/mydesk/internal/linker"
)

func TestDetectNone(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.conf")
	os.WriteFile(srcFile, []byte("content"), 0644)

	dstFile := filepath.Join(dstDir, "test.conf")
	os.Symlink(srcFile, dstFile)

	entries := []linker.LinkEntry{{
		SrcAbs: srcFile,
		DstAbs: dstFile,
	}}

	results := Detect(entries)
	if len(results) != 0 {
		t.Errorf("expected no drift, got %d results", len(results))
	}
}

func TestDetectMissing(t *testing.T) {
	entries := []linker.LinkEntry{{
		Source: "test.conf",
		Target: "~/.test",
		SrcAbs: "/src/test.conf",
		DstAbs: filepath.Join(t.TempDir(), "nonexistent"),
	}}

	results := Detect(entries)
	if len(results) != 1 || results[0].Type != DriftMissing {
		t.Errorf("expected DriftMissing, got %v", results)
	}
}

func TestDetectNotSymlink(t *testing.T) {
	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "test.conf")
	os.WriteFile(dstFile, []byte("regular file"), 0644)

	entries := []linker.LinkEntry{{
		SrcAbs: "/src/test.conf",
		DstAbs: dstFile,
	}}

	results := Detect(entries)
	if len(results) != 1 || results[0].Type != DriftNotSymlink {
		t.Errorf("expected DriftNotSymlink, got %v", results)
	}
}

func TestDetectWrongTarget(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	wrongSrc := filepath.Join(srcDir, "wrong.conf")
	os.WriteFile(wrongSrc, []byte("wrong"), 0644)

	dstFile := filepath.Join(dstDir, "test.conf")
	os.Symlink(wrongSrc, dstFile)

	entries := []linker.LinkEntry{{
		SrcAbs: filepath.Join(srcDir, "correct.conf"),
		DstAbs: dstFile,
	}}

	results := Detect(entries)
	if len(results) != 1 || results[0].Type != DriftWrongTarget {
		t.Errorf("expected DriftWrongTarget, got %v", results)
	}
}

func TestDetectBrokenSymlink(t *testing.T) {
	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "test.conf")

	// Create symlink to nonexistent source
	srcFile := filepath.Join(t.TempDir(), "deleted.conf")
	os.Symlink(srcFile, dstFile)

	entries := []linker.LinkEntry{{
		SrcAbs: srcFile,
		DstAbs: dstFile,
	}}

	results := Detect(entries)
	if len(results) != 1 || results[0].Type != DriftBroken {
		t.Errorf("expected DriftBroken, got %v", results)
	}
}
