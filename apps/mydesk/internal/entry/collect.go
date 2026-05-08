package entry

import (
	"fmt"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/linker"
	"github.com/silee-tools/mydesk/internal/native"
)

// Collect scans native directories and parses links.conf,
// returning the merged list of link entries.
func Collect(cfg *config.Config) ([]linker.LinkEntry, error) {
	nativeEntries, err := native.Scan(cfg)
	if err != nil {
		return nil, fmt.Errorf("scanning native directories: %w", err)
	}

	adapter := &configAdapter{cfg}
	customEntries, err := linker.ParseLinksConf(cfg.LinksConfPath(), adapter)
	if err != nil {
		return nil, fmt.Errorf("parsing links.conf: %w", err)
	}

	return linker.MergeEntries(nativeEntries, customEntries), nil
}

// configAdapter adapts config.Config to linker.ConfigDirProvider.
type configAdapter struct {
	cfg *config.Config
}

func (a *configAdapter) ExpandVars(path string) string {
	return a.cfg.ExpandVars(path)
}

func (a *configAdapter) GetConfigDir() string {
	return a.cfg.ConfigDir
}
