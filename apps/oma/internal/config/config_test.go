package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validTOML = `jira_base_url = "https://jira.example.com"
default_project = "ABC"
product_type_field = "customfield_10001"
start_date_field = "customfield_10002"

[product_type_options]
feature = "Feature"
bug = "Bug"
`

func TestLoadSelectsConfigurationSource(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, paths Paths)
		wantSource Source
		wantErr    string
	}{
		{
			name: "canonical takes precedence when legacy links to it",
			setup: func(t *testing.T, paths Paths) {
				writeFile(t, paths.Canonical, validTOML, 0o600)
				mustMkdirAll(t, filepath.Dir(paths.Legacy), 0o700)
				if err := os.Symlink(paths.Canonical, paths.Legacy); err != nil {
					t.Fatal(err)
				}
			},
			wantSource: SourceCanonical,
		},
		{
			name: "legacy regular file is readable",
			setup: func(t *testing.T, paths Paths) {
				writeFile(t, paths.Legacy, validTOML, 0o600)
			},
			wantSource: SourceLegacy,
		},
		{
			name: "legacy symlink is readable without canonical",
			setup: func(t *testing.T, paths Paths) {
				target := filepath.Join(filepath.Dir(paths.Legacy), "shared.toml")
				writeFile(t, target, validTOML, 0o600)
				if err := os.Symlink(target, paths.Legacy); err != nil {
					t.Fatal(err)
				}
			},
			wantSource: SourceLegacy,
		},
		{
			name: "different regular files conflict",
			setup: func(t *testing.T, paths Paths) {
				writeFile(t, paths.Canonical, validTOML, 0o600)
				writeFile(t, paths.Legacy, validTOML, 0o600)
			},
			wantErr: "configuration conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			tt.setup(t, paths)
			got, source, err := Load(paths)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if source != tt.wantSource {
				t.Fatalf("source = %q, want %q", source, tt.wantSource)
			}
			if got.JiraBaseURL != "https://jira.example.com" || got.ProductTypeOptions["feature"] != "Feature" {
				t.Fatalf("unexpected config: %#v", got)
			}
		})
	}
}

func TestLoadRejectsInvalidTOML(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Canonical, "jira_base_url = [", 0o600)
	if _, _, err := Load(paths); err == nil {
		t.Fatal("Load() error = nil, want TOML decoding error")
	}
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{
		Canonical: filepath.Join(root, "config", "oma", "config.toml"),
		Legacy:    filepath.Join(root, "config", "prep-task", "config.toml"),
		CacheRoot: filepath.Join(root, "cache", "oma"),
		StateRoot: filepath.Join(root, "state", "oma"),
		Netrc:     filepath.Join(root, "home", ".netrc"),
	}
}

func mustMkdirAll(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, mode); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
