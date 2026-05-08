package cmd

import (
	"fmt"
	"os"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/drift"
	"github.com/silee-tools/mydesk/internal/entry"
	"github.com/silee-tools/mydesk/internal/ui"
)

func RunDiff(opts GlobalOpts) error {
	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	entries, err := entry.Collect(cfg)
	if err != nil {
		return err
	}

	results := drift.Detect(entries)

	if len(results) == 0 {
		ui.Success("All %d links are in sync", len(entries))
		return nil
	}

	fmt.Println()
	for _, r := range results {
		switch r.Type {
		case drift.DriftMissing:
			ui.Warning("%s -> %s (%s)", r.Entry.Source, r.Entry.Target, r.Type)
		case drift.DriftBroken:
			ui.Error("%s -> %s (%s: %s)", r.Entry.Source, r.Entry.Target, r.Type, r.Detail)
		case drift.DriftWrongTarget:
			ui.Error("%s -> %s (%s: %s)", r.Entry.Source, r.Entry.Target, r.Type, r.Detail)
		case drift.DriftNotSymlink:
			ui.Warning("%s -> %s (%s)", r.Entry.Source, r.Entry.Target, r.Type)
		}
	}

	fmt.Println()
	ui.Info("%d of %d links have drift", len(results), len(entries))

	os.Exit(1)
	return nil
}
