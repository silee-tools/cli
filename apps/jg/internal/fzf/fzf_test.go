package fzf

import "testing"

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
