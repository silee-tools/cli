package provision

import (
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/ui"
)

func OMZInstall(configDir string, runner *exec.Runner) error {
	home, _ := os.UserHomeDir()
	omzDir := filepath.Join(home, ".oh-my-zsh")

	if _, err := os.Stat(omzDir); os.IsNotExist(err) {
		ui.Info("Installing Oh-My-Zsh...")
		if err := runner.Run("sh", "-c",
			`RUNZSH=no KEEP_ZSHRC=yes sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)"`); err != nil {
			return err
		}
	}

	script := filepath.Join(configDir, "omz", "install.sh")
	if _, err := os.Stat(script); err == nil {
		ui.Info("Running omz/install.sh...")
		return runner.RunScript(script)
	}

	return nil
}
