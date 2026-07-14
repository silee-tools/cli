package config

import "path/filepath"

type Paths struct {
	Canonical string
	Legacy    string
	CacheRoot string
	StateRoot string
	Netrc     string
}

func ResolvePaths(getenv func(string) string, home string) Paths {
	if home == "" {
		home = getenv("HOME")
	}
	configRoot := getenv("XDG_CONFIG_HOME")
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}
	cacheRoot := getenv("XDG_CACHE_HOME")
	if cacheRoot == "" {
		cacheRoot = filepath.Join(home, ".cache")
	}
	stateRoot := getenv("XDG_STATE_HOME")
	if stateRoot == "" {
		stateRoot = filepath.Join(home, ".local", "state")
	}

	return Paths{
		Canonical: filepath.Join(configRoot, "oma", "config.toml"),
		Legacy:    filepath.Join(configRoot, "prep-task", "config.toml"),
		CacheRoot: filepath.Join(cacheRoot, "oma"),
		StateRoot: filepath.Join(stateRoot, "oma"),
		Netrc:     filepath.Join(home, ".netrc"),
	}
}
