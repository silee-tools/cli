package provision

import (
	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/ui"
)

func MiseInstall(runner *exec.Runner) error {
	ui.Info("Running mise install...")
	return runner.Run("mise", "install")
}
