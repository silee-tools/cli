package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/shell"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunInstallShell(opts GlobalOpts) error {
	// Interactive config-dir selection when not specified
	if opts.ConfigDir == "" && os.Getenv("MYDESK_CONFIG_DIR") == "" {
		if !ui.IsInteractive() {
			return fmt.Errorf("install-shell requires --config-dir or MYDESK_CONFIG_DIR.\n" +
				"Example: mydesk --config-dir ~/my-dotfiles install-shell")
		}

		dir, err := promptConfigDir()
		if err != nil {
			return err
		}
		opts.ConfigDir = dir
	}

	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	profilePath := shell.DetectProfile()

	// Resolve symlinks to edit the actual file
	realPath, err := filepath.EvalSymlinks(profilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot resolve %s: %w", profilePath, err)
	}
	if err == nil && realPath != profilePath {
		ui.Info("Resolved %s → %s (symlink)", filepath.Base(profilePath), realPath)
		profilePath = realPath
	}

	// Build block content
	var lines []string
	lines = append(lines, fmt.Sprintf("export MYDESK_CONFIG_DIR=%q", cfg.ConfigDir))

	gobin := resolveGoBinPath()
	if gobin != "" && !isInPath(gobin) {
		lines = append(lines, fmt.Sprintf("export PATH=\"$PATH:%s\"", gobin))
	}

	blockContent := strings.Join(lines, "\n")

	// Preview changes
	if !opts.DryRun {
		fmt.Println()
		ui.Info("Shell profile: %s", ui.Bold(profilePath))
		fmt.Println()
		fmt.Printf("  Will write:\n")
		fmt.Printf("    %s\n", ui.Dim(shell.StartMarker))
		for _, line := range lines {
			fmt.Printf("    %s\n", line)
		}
		fmt.Printf("    %s\n", ui.Dim(shell.EndMarker))

		if ui.IsInteractive() {
			ok, err := ui.Confirm("Proceed?", true)
			if err != nil {
				return err
			}
			if !ok {
				ui.Info("Cancelled")
				return nil
			}
		}
	}

	if opts.DryRun {
		ui.DryRun("would write to %s:", profilePath)
		ui.DryRun("  %s", shell.StartMarker)
		for _, line := range lines {
			ui.DryRun("  %s", line)
		}
		ui.DryRun("  %s", shell.EndMarker)
		return nil
	}

	// Read existing content
	existing, err := os.ReadFile(profilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", profilePath, err)
	}

	newContent := shell.UpsertBlock(string(existing), shell.StartMarker, shell.EndMarker, blockContent)

	// Preserve file permissions
	mode := os.FileMode(0644)
	if info, err := os.Stat(profilePath); err == nil {
		mode = info.Mode()
	}

	if err := os.WriteFile(profilePath, []byte(newContent), mode); err != nil {
		return fmt.Errorf("cannot write %s: %w", profilePath, err)
	}

	ui.Success("Shell profile updated: %s", profilePath)
	ui.Info("Run: source %s", profilePath)

	return nil
}

// promptConfigDir interactively asks the user to select a config directory.
func promptConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	candidates := discoverConfigCandidates(home)

	manualOption := "Enter path manually"
	options := append(candidates, manualOption)

	idx, err := ui.SelectOption("Select config directory:", options, 0)
	if err != nil {
		return "", err
	}

	if idx < 0 || idx >= len(options) {
		return "", fmt.Errorf("invalid selection")
	}

	if options[idx] == manualOption {
		dir, err := ui.ReadLine("Enter config directory path: ")
		if err != nil {
			return "", err
		}
		if dir == "" {
			return "", fmt.Errorf("no path provided")
		}
		// Expand ~ prefix
		if strings.HasPrefix(dir, "~/") {
			dir = filepath.Join(home, dir[2:])
		}
		return dir, nil
	}

	return options[idx], nil
}

// discoverConfigCandidates finds potential config directories.
func discoverConfigCandidates(home string) []string {
	var candidates []string
	seen := map[string]bool{}

	add := func(dir string) {
		if !seen[dir] {
			seen[dir] = true
			candidates = append(candidates, dir)
		}
	}

	// 1. CWD upward search for links.conf
	if dir, err := os.Getwd(); err == nil {
		for {
			if _, err := os.Stat(filepath.Join(dir, "links.conf")); err == nil {
				add(dir)
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 2. Default config dir
	defDir := filepath.Join(home, ".config", "mydesk")
	if _, err := os.Stat(defDir); err == nil {
		add(defDir)
	}

	// 3. Scan ~/Repositories for links.conf (1-level depth)
	reposDir := filepath.Join(home, "Repositories")
	if entries, err := os.ReadDir(reposDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Check direct child
			candidate := filepath.Join(reposDir, e.Name())
			if _, err := os.Stat(filepath.Join(candidate, "links.conf")); err == nil {
				add(candidate)
				continue
			}
			// Check one level deeper (org/repo pattern)
			if subEntries, err := os.ReadDir(candidate); err == nil {
				for _, sub := range subEntries {
					if !sub.IsDir() {
						continue
					}
					subCandidate := filepath.Join(candidate, sub.Name())
					if _, err := os.Stat(filepath.Join(subCandidate, "links.conf")); err == nil {
						add(subCandidate)
					}
				}
			}
		}
	}

	return candidates
}

// resolveGoBinPath returns the Go binary install directory.
func resolveGoBinPath() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		return filepath.Join(gopath, "bin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "go", "bin")
}

// isInPath checks if dir is already in the PATH.
func isInPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}
