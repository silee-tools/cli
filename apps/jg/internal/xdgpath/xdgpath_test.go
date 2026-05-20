package xdgpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDirUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	t.Setenv("HOME", "/home/test")

	got := StateDir("jg")
	want := "/tmp/xdg-state/jg"
	if got != want {
		t.Errorf("StateDir = %q, want %q", got, want)
	}
}

func TestStateDirFallsBackToHomeLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/test")

	got := StateDir("jg")
	want := filepath.Join("/home/test", ".local/state/jg")
	if got != want {
		t.Errorf("StateDir = %q, want %q", got, want)
	}
}

func TestStateDirSkipsEmptyTool(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	got := StateDir("")
	if got != "/tmp/xdg-state" {
		t.Errorf("StateDir(\"\") = %q, want %q", got, "/tmp/xdg-state")
	}
	_ = os.Getenv
}
