package cmd

import (
	"fmt"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/provision"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunSync(opts GlobalOpts) error {
	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	runner := exec.New(opts.DryRun, opts.Verbose)
	var failed int

	ui.Info("Syncing current system state to %s", cfg.ConfigDir)
	fmt.Println()

	// Brewfile
	ui.Info("Syncing Brewfile...")
	if err := provision.BrewSync(cfg.ConfigDir, runner); err != nil {
		ui.Error("brew sync: %v", err)
		failed++
	} else {
		ui.Success("brew/Brewfile updated")
	}

	// VS Code extensions
	ui.Info("Syncing VS Code extensions...")
	if err := provision.VSCodeSync(cfg.ConfigDir, runner); err != nil {
		ui.Error("vscode sync: %v", err)
		failed++
	} else {
		ui.Success("vscode/extensions.txt updated")
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d sync task(s) failed", failed)
	}
	ui.Success("Sync complete")
	return nil
}
