package linker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LinkEntry struct {
	Source     string // raw source from links.conf or native scanner
	Target     string // raw target
	SrcAbs     string // expanded absolute source path
	DstAbs     string // expanded absolute target path
	IsExternal bool   // source is outside the config repo
	Line       int    // line number (0 for native entries)
}

type VarExpander interface {
	ExpandVars(path string) string
}

type ConfigDirProvider interface {
	VarExpander
	GetConfigDir() string
}

func ParseLinksConf(path string, cfg ConfigDirProvider) ([]LinkEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // links.conf is optional
		}
		return nil, err
	}
	defer f.Close()

	var entries []LinkEntry
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "->", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("links.conf:%d: invalid format, expected 'SOURCE -> TARGET'", lineNum)
		}

		source := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])

		srcExpanded := cfg.ExpandVars(source)
		dstExpanded := cfg.ExpandVars(target)

		isExternal := filepath.IsAbs(srcExpanded) || strings.HasPrefix(source, "$")
		if !isExternal {
			srcExpanded = filepath.Join(cfg.GetConfigDir(), srcExpanded)
		}

		entries = append(entries, LinkEntry{
			Source:     source,
			Target:     target,
			SrcAbs:     srcExpanded,
			DstAbs:     dstExpanded,
			IsExternal: isExternal,
			Line:       lineNum,
		})
	}

	return entries, scanner.Err()
}

func MergeEntries(native, custom []LinkEntry) []LinkEntry {
	// custom (links.conf) entries override native entries with the same DstAbs
	dstSet := make(map[string]bool)
	for _, e := range custom {
		dstSet[e.DstAbs] = true
	}

	var merged []LinkEntry
	for _, e := range native {
		if !dstSet[e.DstAbs] {
			merged = append(merged, e)
		}
	}
	merged = append(merged, custom...)
	return merged
}
