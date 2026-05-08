package cmd

import (
	"fmt"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/provision"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunSetup(opts GlobalOpts) error {
	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	runner := exec.New(opts.DryRun, opts.Verbose)
	var failed int

	steps := []struct {
		name string
		fn   func() error
	}{
		{"Homebrew packages", func() error { return provision.BrewInstall(cfg.ConfigDir, runner) }},
		{"Oh-My-Zsh", func() error { return provision.OMZInstall(cfg.ConfigDir, runner) }},
		{"mise runtimes", func() error { return provision.MiseInstall(runner) }},
		{"Symlinks", func() error { return RunLink(opts) }},
		{"Shell profile", func() error {
			setupOpts := opts
			setupOpts.ConfigDir = cfg.ConfigDir
			return RunInstallShell(setupOpts)
		}},
		{"macOS defaults", func() error { return provision.ApplyDefaults(cfg.ConfigDir, runner) }},
		{"VS Code extensions", func() error { return provision.VSCodeInstall(cfg.ConfigDir, runner) }},
	}

	if !opts.DryRun && ui.IsInteractive() {
		fmt.Println()
		ui.Info("Full provisioning (%d steps):", len(steps))
		for i, step := range steps {
			fmt.Printf("    %d. %s\n", i+1, step.name)
		}

		ok, err := ui.Confirm("Proceed?", true)
		if err != nil {
			return err
		}
		if !ok {
			ui.Info("Cancelled")
			return nil
		}
	}

	for i, step := range steps {
		fmt.Println()
		ui.Info("[%d/%d] %s", i+1, len(steps), ui.Bold(step.name))

		if err := step.fn(); err != nil {
			ui.Error("%s: %v", step.name, err)
			failed++
		} else {
			ui.Success("%s done", step.name)
		}
	}

	fmt.Println()
	if failed > 0 {
		ui.Warning("Setup completed with %d error(s)", failed)
		return fmt.Errorf("%d setup step(s) failed", failed)
	}
	ui.Success("Setup complete!")
	return nil
}
