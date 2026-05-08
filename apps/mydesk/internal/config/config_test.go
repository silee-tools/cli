package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandVars(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/test/config",
		Vars: map[string]string{
			"HOME":  "/Users/testuser",
			"REPOS": "/Users/testuser/Repositories",
		},
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~", "/Users/testuser"},
		{"~/", "/Users/testuser/"},
		{"~/.zshrc", "/Users/testuser/.zshrc"},
		{"~/.config/ghostty/config", "/Users/testuser/.config/ghostty/config"},
		{"$HOME/.zshrc", "/Users/testuser/.zshrc"},
		{"$REPOS/org/repo", "/Users/testuser/Repositories/org/repo"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cfg.ExpandVars(tt.input)
			if got != tt.want {
				t.Errorf("ExpandVars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindUpward(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	// Create links.conf at root
	confPath := filepath.Join(tmpDir, LinksConfFile)
	if err := os.WriteFile(confPath, []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save and restore CWD
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(nested)
	dir, ok := findUpward(LinksConfFile)
	if !ok {
		t.Fatal("findUpward should find links.conf")
	}
	// Resolve symlinks for comparison (macOS /var -> /private/var)
	wantResolved, _ := filepath.EvalSymlinks(tmpDir)
	gotResolved, _ := filepath.EvalSymlinks(dir)
	if gotResolved != wantResolved {
		t.Errorf("findUpward found %q, want %q", gotResolved, wantResolved)
	}
}

func TestValidateConfigDir(t *testing.T) {
	home := "/Users/testuser"

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"home dir blocked", home, true},
		{"dotconfig blocked", filepath.Join(home, ".config"), true},
		{"ssh blocked", filepath.Join(home, ".ssh"), true},
		{"vscode blocked", filepath.Join(home, "Library", "Application Support", "Code", "User"), true},
		{"subdirectory ok", filepath.Join(home, ".config", "mydesk"), false},
		{"other path ok", "/tmp/mydesk-config", false},
		{"home subdir ok", filepath.Join(home, "dotfiles"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigDir(tt.dir, home)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfigDir(%q) error = %v, wantErr %v", tt.dir, err, tt.wantErr)
			}
		})
	}
}

func TestFindUpwardNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(tmpDir)
	_, ok := findUpward("nonexistent-file-xyz")
	if ok {
		t.Error("findUpward should not find nonexistent file")
	}
}
