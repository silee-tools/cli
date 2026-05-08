package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silee-tools/mydesk/internal/ui"
)

var initDirs = []string{
	"home",
	"config",
	"ssh",
	"vscode",
	"brew",
	"macos",
	"omz",
}

const linksConfTemplate = `# mydesk links.conf
# Format: SOURCE -> TARGET
# Relative paths are resolved from this directory.
# Use $REPOS, $HOME, ~ for variable expansion.

# Example: external repo symlinks
# $REPOS/my-org/some-repo/config -> ~/.some-config
`

const gitignoreTemplate = `# SSH keys (only config is tracked)
ssh/id_*
ssh/*.pem
ssh/known_hosts
ssh/authorized_keys

# Environment secrets
.env
.env.*

# Shell history
.zsh_history
.bash_history

# OS artifacts
.DS_Store
._*
.Spotlight-V100
.Trashes
`

const readmeTemplate = `# My Dotfiles

Managed by [mydesk](https://github.com/silee-tools/mydesk).

## Usage

` + "```bash" + `
# Link all config files
mydesk link

# Check for drift
mydesk diff

# Sync current state (Brewfile, VS Code extensions)
mydesk sync

# Full provisioning on a new machine
mydesk setup
` + "```" + `
`

func RunInit(opts GlobalOpts, targetDir string) error {
	if targetDir == "" {
		if !ui.IsInteractive() {
			return fmt.Errorf("target directory is required: mydesk init <directory>")
		}
		dir, err := ui.ReadLine("Enter config directory path [~/my-dotfiles]: ")
		if err != nil {
			return err
		}
		if dir == "" {
			dir = "~/my-dotfiles"
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		if strings.HasPrefix(dir, "~/") {
			dir = filepath.Join(home, dir[2:])
		}
		targetDir = dir
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(absDir, "links.conf")); err == nil {
		return fmt.Errorf("already initialized: %s/links.conf exists", absDir)
	}

	if opts.DryRun {
		ui.DryRun("would create config repo at %s", absDir)
		for _, d := range initDirs {
			ui.DryRun("  mkdir %s/", d)
		}
		ui.DryRun("  create links.conf")
		ui.DryRun("  create .gitignore")
		ui.DryRun("  create README.md")
		return nil
	}

	// Create directories
	for _, d := range initDirs {
		dir := filepath.Join(absDir, d)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		// Add .gitkeep to empty dirs
		gitkeep := filepath.Join(dir, ".gitkeep")
		if err := os.WriteFile(gitkeep, []byte{}, 0644); err != nil {
			return err
		}
	}

	// Create files
	files := map[string]string{
		"links.conf": linksConfTemplate,
		".gitignore": gitignoreTemplate,
		"README.md":  readmeTemplate,
	}

	for name, content := range files {
		path := filepath.Join(absDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	ui.Success("Initialized config repo at %s", absDir)
	ui.Info("Next steps:")
	fmt.Println("  1. Add your dotfiles to home/, config/, etc.")
	fmt.Println("  2. Run 'mydesk link' to create symlinks")
	fmt.Println("  3. Run 'mydesk sync' to export Brewfile and VS Code extensions")

	return nil
}
