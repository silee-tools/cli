package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	LinksConfFile      = "links.conf"
	DefaultConfigDir   = "~/.config/mydesk"
	EnvConfigDir       = "MYDESK_CONFIG_DIR"
	EnvRepos           = "MYDESK_REPOS"
)

type Config struct {
	ConfigDir string
	Vars      map[string]string
}

func Load(configDir string) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	dir, err := resolveConfigDir(configDir, home)
	if err != nil {
		return nil, err
	}

	if err := validateConfigDir(dir, home); err != nil {
		return nil, err
	}

	vars := map[string]string{
		"HOME":  home,
		"REPOS": resolveRepos(home),
	}

	return &Config{ConfigDir: dir, Vars: vars}, nil
}

// validateConfigDir rejects config directories that coincide with native
// directory targets. Using e.g. ~/.config as config-dir would cause the
// config/ native directory to create self-referencing symlinks.
func validateConfigDir(dir, home string) error {
	forbidden := []struct {
		path  string
		label string
	}{
		{home, "~ (home/ native dir target)"},
		{filepath.Join(home, ".config"), "~/.config (config/ native dir target)"},
		{filepath.Join(home, ".ssh"), "~/.ssh (ssh/ native dir target)"},
		{filepath.Join(home, "Library", "Application Support", "Code", "User"), "VS Code User dir (vscode/ native dir target)"},
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot resolve config directory: %w", err)
	}
	if resolved == "" {
		resolved = dir
	}

	for _, f := range forbidden {
		if dir == f.path || resolved == f.path {
			return fmt.Errorf("config directory must not be %s.\nThis would cause self-referencing symlinks. Use a subdirectory instead (e.g. %s/mydesk)",
				f.label, f.path)
		}
	}
	return nil
}

func (c *Config) ExpandVars(path string) string {
	if path == "~" {
		return c.Vars["HOME"]
	}
	if strings.HasPrefix(path, "~/") {
		path = c.Vars["HOME"] + path[1:]
	}
	for k, v := range c.Vars {
		path = strings.ReplaceAll(path, "$"+k, v)
	}
	return path
}

func (c *Config) LinksConfPath() string {
	return filepath.Join(c.ConfigDir, LinksConfFile)
}

func resolveConfigDir(explicit string, home string) (string, error) {
	// 1. explicit flag
	if explicit != "" {
		return expandHome(explicit, home), nil
	}

	// 2. environment variable
	if env := os.Getenv(EnvConfigDir); env != "" {
		return expandHome(env, home), nil
	}

	// 3. walk up from CWD looking for links.conf
	if dir, ok := findUpward(LinksConfFile); ok {
		return dir, nil
	}

	// 4. default
	def := expandHome(DefaultConfigDir, home)
	if _, err := os.Stat(def); err == nil {
		return def, nil
	}

	return "", fmt.Errorf("cannot find config directory: no %s found in parent directories, and %s does not exist.\nUse --config-dir or set %s", LinksConfFile, DefaultConfigDir, EnvConfigDir)
}

func resolveRepos(home string) string {
	if env := os.Getenv(EnvRepos); env != "" {
		return expandHome(env, home)
	}
	return filepath.Join(home, "Repositories")
}

func findUpward(filename string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, filename)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func expandHome(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}
