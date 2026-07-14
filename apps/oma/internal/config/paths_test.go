package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		home string
		want Paths
	}{
		{
			name: "uses each XDG root independently",
			env: map[string]string{
				"XDG_CONFIG_HOME": "/xdg/config",
				"XDG_CACHE_HOME":  "/xdg/cache",
				"XDG_STATE_HOME":  "/xdg/state",
			},
			home: "/home/tester",
			want: Paths{
				Canonical: "/xdg/config/oma/config.toml",
				Legacy:    "/xdg/config/prep-task/config.toml",
				CacheRoot: "/xdg/cache/oma",
				StateRoot: "/xdg/state/oma",
				Netrc:     "/home/tester/.netrc",
			},
		},
		{
			name: "falls back to HOME based roots",
			home: "/home/tester",
			want: Paths{
				Canonical: "/home/tester/.config/oma/config.toml",
				Legacy:    "/home/tester/.config/prep-task/config.toml",
				CacheRoot: "/home/tester/.cache/oma",
				StateRoot: "/home/tester/.local/state/oma",
				Netrc:     "/home/tester/.netrc",
			},
		},
		{
			name: "reads HOME when the explicit home is empty",
			env:  map[string]string{"HOME": "/env/home"},
			want: Paths{
				Canonical: "/env/home/.config/oma/config.toml",
				Legacy:    "/env/home/.config/prep-task/config.toml",
				CacheRoot: "/env/home/.cache/oma",
				StateRoot: "/env/home/.local/state/oma",
				Netrc:     "/env/home/.netrc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			got := ResolvePaths(getenv, tt.home)
			if got != tt.want {
				t.Fatalf("ResolvePaths() = %#v, want %#v", got, tt.want)
			}
			for _, path := range []string{got.Canonical, got.Legacy, got.CacheRoot, got.StateRoot, got.Netrc} {
				if !filepath.IsAbs(path) {
					t.Fatalf("path is not absolute: %q", path)
				}
			}
		})
	}
}
