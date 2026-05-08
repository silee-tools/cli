package provision

import (
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/exec"
)

func BrewSync(configDir string, runner *exec.Runner) error {
	brewfile := filepath.Join(configDir, "brew", "Brewfile")
	return runner.Run("brew", "bundle", "dump", "--force", "--file="+brewfile)
}

func BrewInstall(configDir string, runner *exec.Runner) error {
	brewfile := filepath.Join(configDir, "brew", "Brewfile")
	return runner.Run("brew", "bundle", "install", "--file="+brewfile)
}
