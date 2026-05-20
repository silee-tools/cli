// Package xdgpath returns XDG Base Directory paths with sensible fallbacks
// when the corresponding environment variables are unset.
package xdgpath

import (
	"os"
	"path/filepath"
)

// StateDir returns the directory where the given tool stores state files.
// It honors XDG_STATE_HOME and falls back to ~/.local/state when unset.
// Passing an empty tool name returns the base state dir without a tool suffix.
func StateDir(tool string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	if tool == "" {
		return base
	}
	return filepath.Join(base, tool)
}
