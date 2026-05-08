package cmd

import (
	"fmt"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/entry"
	"github.com/silee-tools/mydesk/internal/linker"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunLink(opts GlobalOpts) error {
	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	entries, err := entry.Collect(cfg)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		ui.Warning("No link entries found")
		return nil
	}

	ui.Info("Linking %d entries from %s", len(entries), cfg.ConfigDir)
	fmt.Println()

	r := linker.New(entries, opts.DryRun).Link()

	fmt.Println()
	ui.Info("Done: %d linked, %d skipped, %d backed up, %d failed",
		r.Linked, r.Skipped, r.BackedUp, r.Failed)

	if r.Failed > 0 {
		return fmt.Errorf("%d link(s) failed", r.Failed)
	}
	return nil
}
