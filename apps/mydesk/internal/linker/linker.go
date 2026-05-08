package linker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/ui"
)

const BackupSuffix = ".mydesk-backup"

type LinkReport struct {
	Linked  int
	Skipped int
	BackedUp int
	Failed  int
	Errors  []error
}

type Linker struct {
	Entries []LinkEntry
	DryRun  bool
}

func New(entries []LinkEntry, dryRun bool) *Linker {
	return &Linker{Entries: entries, DryRun: dryRun}
}

func (l *Linker) Link() LinkReport {
	var r LinkReport

	for _, e := range l.Entries {
		err := l.linkOne(e, &r)
		if err != nil {
			r.Failed++
			r.Errors = append(r.Errors, fmt.Errorf("%s: %w", e.Target, err))
			ui.Error("%s -> %s (%v)", e.Source, e.Target, err)
		}
	}

	return r
}

func (l *Linker) linkOne(e LinkEntry, r *LinkReport) error {
	// check source exists
	if _, err := os.Lstat(e.SrcAbs); err != nil {
		return fmt.Errorf("source not found: %s", e.SrcAbs)
	}

	if l.DryRun {
		ui.DryRun("%s -> %s", e.Source, e.Target)
		r.Linked++
		return nil
	}

	// ensure parent directory
	if err := os.MkdirAll(filepath.Dir(e.DstAbs), 0755); err != nil {
		return err
	}

	// check existing target
	info, err := os.Lstat(e.DstAbs)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// already a symlink
			current, _ := os.Readlink(e.DstAbs)
			if current == e.SrcAbs {
				ui.Dim(fmt.Sprintf(" ⊘ %s -> %s (already linked)", e.Source, e.Target))
				r.Skipped++
				return nil
			}
		}
		// backup existing
		backupPath := e.DstAbs + BackupSuffix
		if _, err := os.Lstat(backupPath); err == nil {
			return fmt.Errorf("backup already exists: %s (remove manually to proceed)", backupPath)
		}
		if err := os.Rename(e.DstAbs, backupPath); err != nil {
			return fmt.Errorf("failed to backup: %w", err)
		}
		r.BackedUp++
		ui.Warning("%s -> %s (backed up existing)", e.Source, e.Target)
	}

	if err := os.Symlink(e.SrcAbs, e.DstAbs); err != nil {
		return err
	}

	ui.Success("%s -> %s", e.Source, e.Target)
	r.Linked++
	return nil
}

func (l *Linker) Unlink() LinkReport {
	var r LinkReport

	for _, e := range l.Entries {
		err := l.unlinkOne(e, &r)
		if err != nil {
			r.Failed++
			r.Errors = append(r.Errors, fmt.Errorf("%s: %w", e.Target, err))
			ui.Error("%s (%v)", e.Target, err)
		}
	}

	return r
}

func (l *Linker) unlinkOne(e LinkEntry, r *LinkReport) error {
	info, err := os.Lstat(e.DstAbs)
	if err != nil {
		r.Skipped++
		return nil // target doesn't exist, nothing to unlink
	}

	if l.DryRun {
		ui.DryRun("unlink %s", e.Target)
		r.Linked++
		return nil
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("not a symlink: %s", e.DstAbs)
	}

	current, _ := os.Readlink(e.DstAbs)
	if current != e.SrcAbs {
		return fmt.Errorf("symlink points to %s, not %s", current, e.SrcAbs)
	}

	if err := os.Remove(e.DstAbs); err != nil {
		return err
	}

	// restore backup if exists
	backupPath := e.DstAbs + BackupSuffix
	if _, err := os.Lstat(backupPath); err == nil {
		if err := os.Rename(backupPath, e.DstAbs); err != nil {
			return fmt.Errorf("failed to restore backup: %w", err)
		}
		ui.Success("unlinked %s (backup restored)", e.Target)
	} else {
		ui.Success("unlinked %s", e.Target)
	}

	r.Linked++
	return nil
}
