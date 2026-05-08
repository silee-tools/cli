package provision

import (
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/ui"
)

func ApplyDefaults(configDir string, runner *exec.Runner) error {
	script := filepath.Join(configDir, "macos", "defaults.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		ui.Warning("macos/defaults.sh not found, skipping")
		return nil
	}

	ui.Info("Applying macOS defaults...")
	return runner.RunScript(script)
}
