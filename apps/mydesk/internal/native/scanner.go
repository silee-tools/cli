package native

import (
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/linker"
)

func Scan(cfg *config.Config) ([]linker.LinkEntry, error) {
	var entries []linker.LinkEntry

	for _, nd := range LinkDirs() {
		srcDir := filepath.Join(cfg.ConfigDir, nd.Dir)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}

		targetBase := cfg.ExpandVars(nd.TargetBase)

		dirEntries, err := os.ReadDir(srcDir)
		if err != nil {
			return nil, err
		}

		for _, e := range dirEntries {
			if e.Name() == ".gitkeep" {
				continue
			}
			srcAbs := filepath.Join(srcDir, e.Name())
			dstAbs := filepath.Join(targetBase, e.Name())

			entries = append(entries, linker.LinkEntry{
				Source:     filepath.Join(nd.Dir, e.Name()),
				Target:     filepath.Join(nd.TargetBase, e.Name()),
				SrcAbs:     srcAbs,
				DstAbs:     dstAbs,
				IsExternal: false,
			})
		}
	}

	return entries, nil
}
