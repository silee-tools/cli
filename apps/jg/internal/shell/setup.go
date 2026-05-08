package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type SetupResult struct {
	Shell   string
	Actions []string
}

func Setup(shellOverride string) (*SetupResult, error) {
	shellName := shellOverride
	if shellName == "" {
		shellName = filepath.Base(os.Getenv("SHELL"))
	}
	if shellName != "zsh" && shellName != "bash" {
		return nil, fmt.Errorf("unsupported shell: %s (supported: zsh, bash)", shellName)
	}

	result := &SetupResult{Shell: shellName}

	switch shellName {
	case "zsh":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		omzDir := os.Getenv("ZSH")
		if omzDir == "" {
			omzDir = filepath.Join(home, ".oh-my-zsh")
		}
		if info, err := os.Stat(omzDir); err == nil && info.IsDir() {
			if err := setupOhMyZsh(home, omzDir, result); err != nil {
				return nil, err
			}
		} else {
			if err := setupPlainZsh(home, result); err != nil {
				return nil, err
			}
		}
	case "bash":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if err := setupBash(home, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func setupOhMyZsh(home, omzDir string, result *SetupResult) error {
	// 1. Find plugin source file
	pluginSrc, err := findPluginFile()
	if err != nil {
		// Fallback to plain zsh if plugin file not found
		return setupPlainZsh(home, result)
	}

	// 2. Create plugin directory
	zshCustom := os.Getenv("ZSH_CUSTOM")
	if zshCustom == "" {
		zshCustom = filepath.Join(omzDir, "custom")
	}
	pluginDir := filepath.Join(zshCustom, "plugins", "jg")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// 3. Create symlink (idempotent)
	linkPath := filepath.Join(pluginDir, "jg.plugin.zsh")
	if target, err := os.Readlink(linkPath); err != nil || target != pluginSrc {
		os.Remove(linkPath)
		if err := os.Symlink(pluginSrc, linkPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("Created symlink %s", linkPath))
	}

	// 4. Add jg to plugins=(...) in .zshrc
	zshrc := filepath.Join(home, ".zshrc")
	content, err := os.ReadFile(zshrc)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	text := string(content)
	if regexp.MustCompile(`(?m)^[^#]*plugins\s*=\s*\([^)]*\bjg\b`).MatchString(text) {
		return nil // already configured
	}

	pluginsRe := regexp.MustCompile(`(?m)(^plugins\s*=\s*\()`)
	if !pluginsRe.MatchString(text) {
		result.Actions = append(result.Actions, "Could not find plugins=(...) in .zshrc. Add 'jg' to your plugins manually.")
		return nil
	}

	newText := pluginsRe.ReplaceAllStringFunc(text, func(match string) string {
		return match + "\n  jg"
	})
	if err := os.WriteFile(zshrc, []byte(newText), 0644); err != nil {
		return fmt.Errorf("failed to update .zshrc: %w", err)
	}
	result.Actions = append(result.Actions, "Added 'jg' to plugins in ~/.zshrc")

	return nil
}

func setupPlainZsh(home string, result *SetupResult) error {
	return appendEvalLine(filepath.Join(home, ".zshrc"), `eval "$(jg init zsh)"`, result)
}

func setupBash(home string, result *SetupResult) error {
	return appendEvalLine(filepath.Join(home, ".bashrc"), `eval "$(jg init bash)"`, result)
}

func appendEvalLine(rcFile, evalLine string, result *SetupResult) error {
	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if strings.Contains(string(content), evalLine) {
		return nil // already configured
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", rcFile, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n%s\n", evalLine); err != nil {
		return err
	}

	result.Actions = append(result.Actions, fmt.Sprintf("Added '%s' to %s", evalLine, filepath.Base(rcFile)))
	return nil
}

func findPluginFile() (string, error) {
	// Use LookPath to get the unresolved symlink path (e.g., /opt/homebrew/bin/jg)
	// so the plugin symlink survives brew upgrades.
	// os.Executable() resolves symlinks on macOS, pointing to Cellar which changes per version.
	exe, err := exec.LookPath("jg")
	if err != nil {
		return "", err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", err
	}

	pluginPath := filepath.Join(filepath.Dir(exe), "..", "share", "jg", "plugin", "jg.plugin.zsh")
	pluginPath, err = filepath.Abs(pluginPath)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(pluginPath); err != nil {
		return "", fmt.Errorf("plugin file not found at %s", pluginPath)
	}

	return pluginPath, nil
}
