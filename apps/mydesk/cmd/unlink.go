package cmd

import (
	"fmt"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/entry"
	"github.com/silee-tools/mydesk/internal/linker"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunUnlink(opts GlobalOpts) error {
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

	ui.Info("Unlinking %d entries", len(entries))

	if !opts.DryRun && ui.IsInteractive() {
		ok, err := ui.Confirm(fmt.Sprintf("Will unlink %d entries and restore backups. Continue?", len(entries)), false)
		if err != nil {
			return err
		}
		if !ok {
			ui.Info("Cancelled")
			return nil
		}
	}

	fmt.Println()

	r := linker.New(entries, opts.DryRun).Unlink()

	fmt.Println()
	ui.Info("Done: %d unlinked, %d skipped, %d failed",
		r.Linked, r.Skipped, r.Failed)

	if r.Failed > 0 {
		return fmt.Errorf("%d unlink(s) failed", r.Failed)
	}
	return nil
}
