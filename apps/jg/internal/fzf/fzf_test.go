package fzf

import (
	"testing"

	"github.com/silee-tools/jg/internal/entry"
)

func TestShortenPath(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"/Users/silee/repos/jg", "/Users/silee", "~/repos/jg"},
		{"/opt/other/path", "/Users/silee", "/opt/other/path"},
		{"/Users/silee2/repos/jg", "/Users/silee", "/Users/silee2/repos/jg"},
		{"/Users/silee", "/Users/silee", "~"},
		{"~/already-short", "/Users/silee", "~/already-short"},
		{"/some/path", "", "/some/path"},
	}
	for _, tt := range tests {
		if got := shortenPath(tt.path, tt.home); got != tt.want {
			t.Errorf("shortenPath(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"~/repos/jg", "/Users/silee", "/Users/silee/repos/jg"},
		{"/opt/other/path", "/Users/silee", "/opt/other/path"},
		{"~", "/Users/silee", "~"},
		{"~/repos/jg", "", "~/repos/jg"},
	}
	for _, tt := range tests {
		if got := expandPath(tt.path, tt.home); got != tt.want {
			t.Errorf("expandPath(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

func TestBuildPickerLinesPinsMainWithLabel(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{
		{Path: "/home/tester/repos/a"},
		{Path: "/home/tester/repos/b"},
	}
	lines := buildPickerLines(entries, "/home/tester/repos/main", home)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	if lines[0].display != "↑ main  ~/repos/main" {
		t.Errorf("pinned display = %q", lines[0].display)
	}
	if lines[0].pathField != "~/repos/main" {
		t.Errorf("pinned pathField = %q", lines[0].pathField)
	}
}

func TestBuildPickerLinesDedupsPinnedFromBody(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{
		{Path: "/home/tester/repos/main"},
		{Path: "/home/tester/repos/a"},
	}
	lines := buildPickerLines(entries, "/home/tester/repos/main", home)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (pin + a, main deduped), got %d", len(lines))
	}
	for i, ln := range lines[1:] {
		if ln.pathField == "~/repos/main" {
			t.Errorf("main appeared again in body at index %d", i)
		}
	}
}

func TestBuildPickerLinesNoPin(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{{Path: "/home/tester/repos/a"}}
	lines := buildPickerLines(entries, "", home)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	if lines[0].display != "~/repos/a" || lines[0].pathField != "~/repos/a" {
		t.Errorf("no-pin line = %+v", lines[0])
	}
}

func TestParseSelectedPath(t *testing.T) {
	home := "/home/tester"
	tests := []struct{ in, want string }{
		{"↑ main  ~/repos/main\t~/repos/main", "/home/tester/repos/main"},
		{"~/repos/a\t~/repos/a", "/home/tester/repos/a"},
		{"/opt/x\t/opt/x", "/opt/x"},
		{"~/repos/a", "/home/tester/repos/a"},
		{"", ""},
		{"  ~/repos/a\t~/repos/a  ", "/home/tester/repos/a"},
	}
	for _, tt := range tests {
		if got := parseSelectedPath(tt.in, home); got != tt.want {
			t.Errorf("parseSelectedPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
