package linker

import (
	"os"
	"path/filepath"
	"testing"
)

type mockConfig struct {
	configDir string
	vars      map[string]string
}

func (m *mockConfig) ExpandVars(path string) string {
	if path == "~" {
		return m.vars["HOME"]
	}
	if len(path) > 1 && path[:2] == "~/" {
		path = m.vars["HOME"] + path[1:]
	}
	for k, v := range m.vars {
		for {
			replaced := path
			idx := 0
			for idx < len(replaced) {
				if replaced[idx] == '$' {
					rest := replaced[idx+1:]
					if len(rest) >= len(k) && rest[:len(k)] == k {
						replaced = replaced[:idx] + v + rest[len(k):]
						break
					}
				}
				idx++
			}
			if replaced == path {
				break
			}
			path = replaced
		}
	}
	return path
}

func (m *mockConfig) GetConfigDir() string {
	return m.configDir
}

func TestParseLinksConf(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "links.conf")

	content := `# Comment line
home/.zshrc -> ~/.zshrc

# Another comment
config/ghostty/config -> ~/.config/ghostty/config
$REPOS/org/repo/dir -> ~/.target
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &mockConfig{
		configDir: tmpDir,
		vars: map[string]string{
			"HOME":  "/Users/test",
			"REPOS": "/Users/test/Repos",
		},
	}

	entries, err := ParseLinksConf(confPath, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entry 1: internal source
	e := entries[0]
	if e.Source != "home/.zshrc" {
		t.Errorf("entry 0 source = %q", e.Source)
	}
	if e.SrcAbs != filepath.Join(tmpDir, "home/.zshrc") {
		t.Errorf("entry 0 srcAbs = %q", e.SrcAbs)
	}
	if e.DstAbs != "/Users/test/.zshrc" {
		t.Errorf("entry 0 dstAbs = %q", e.DstAbs)
	}
	if e.IsExternal {
		t.Error("entry 0 should not be external")
	}

	// Entry 2: internal source with nested path
	e = entries[1]
	if e.DstAbs != "/Users/test/.config/ghostty/config" {
		t.Errorf("entry 1 dstAbs = %q", e.DstAbs)
	}

	// Entry 3: external source ($REPOS)
	e = entries[2]
	if !e.IsExternal {
		t.Error("entry 2 should be external")
	}
	if e.SrcAbs != "/Users/test/Repos/org/repo/dir" {
		t.Errorf("entry 2 srcAbs = %q", e.SrcAbs)
	}
}

func TestParseLinksConfMissing(t *testing.T) {
	cfg := &mockConfig{configDir: t.TempDir()}
	entries, err := ParseLinksConf("/nonexistent/links.conf", cfg)
	if err != nil {
		t.Fatal("missing links.conf should not error")
	}
	if entries != nil {
		t.Error("missing links.conf should return nil")
	}
}

func TestParseLinksConfInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "links.conf")
	if err := os.WriteFile(confPath, []byte("no arrow here\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &mockConfig{configDir: tmpDir}
	_, err := ParseLinksConf(confPath, cfg)
	if err == nil {
		t.Error("should error on invalid format")
	}
}

func TestMergeEntries(t *testing.T) {
	native := []LinkEntry{
		{Source: "home/.zshrc", DstAbs: "/home/.zshrc"},
		{Source: "home/.gitconfig", DstAbs: "/home/.gitconfig"},
	}
	custom := []LinkEntry{
		{Source: "custom/.zshrc", DstAbs: "/home/.zshrc"},
		{Source: "extra/file", DstAbs: "/extra/file"},
	}

	merged := MergeEntries(native, custom)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged entries, got %d", len(merged))
	}

	// .gitconfig from native (not overridden)
	if merged[0].Source != "home/.gitconfig" {
		t.Errorf("merged[0] = %q, want native .gitconfig", merged[0].Source)
	}
	// .zshrc from custom (overrides native)
	if merged[1].Source != "custom/.zshrc" {
		t.Errorf("merged[1] = %q, want custom .zshrc", merged[1].Source)
	}
	// extra from custom
	if merged[2].Source != "extra/file" {
		t.Errorf("merged[2] = %q, want extra/file", merged[2].Source)
	}
}
