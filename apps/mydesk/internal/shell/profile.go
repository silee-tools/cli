package shell

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	StartMarker = "# --- mydesk (managed) ---"
	EndMarker   = "# --- end mydesk ---"
)

// DetectProfile returns the login shell profile path based on $SHELL.
func DetectProfile() string {
	home, _ := os.UserHomeDir()
	sh := os.Getenv("SHELL")

	switch filepath.Base(sh) {
	case "zsh":
		return filepath.Join(home, ".zprofile")
	case "bash":
		// Prefer .bash_profile if it exists, otherwise .profile
		bp := filepath.Join(home, ".bash_profile")
		if _, err := os.Stat(bp); err == nil {
			return bp
		}
		return filepath.Join(home, ".profile")
	default:
		return filepath.Join(home, ".profile")
	}
}

// FindBlock returns the byte offsets of the marker block in content.
// Returns (start, end, true) if found, or (0, 0, false) if not.
func FindBlock(content, startMarker, endMarker string) (int, int, bool) {
	startIdx := strings.Index(content, startMarker)
	if startIdx < 0 {
		return 0, 0, false
	}

	endIdx := strings.Index(content[startIdx:], endMarker)
	if endIdx < 0 {
		return 0, 0, false
	}

	return startIdx, startIdx + endIdx + len(endMarker), true
}

// UpsertBlock replaces an existing marker block or appends a new one.
// The newBlock should NOT include the markers — they are added automatically.
func UpsertBlock(content, startMarker, endMarker, newBlock string) string {
	block := startMarker + "\n" + newBlock + "\n" + endMarker

	start, end, found := FindBlock(content, startMarker, endMarker)
	if found {
		// Replace existing block
		// Consume trailing newline if present
		if end < len(content) && content[end] == '\n' {
			end++
		}
		return content[:start] + block + "\n" + content[end:]
	}

	// Append new block
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
		content += "\n"
	}
	return content + block + "\n"
}
