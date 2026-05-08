package web

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/drift"
	"github.com/silee-tools/mydesk/internal/entry"
	"github.com/silee-tools/mydesk/internal/native"
)

type statusResponse struct {
	ConfigDir  string            `json:"configDir"`
	Version    string            `json:"version"`
	Vars       map[string]string `json:"vars"`
	Links      linksSummary      `json:"links"`
	Drift      driftSummary      `json:"drift"`
	NativeDirs []nativeDirInfo   `json:"nativeDirs"`
}

type linksSummary struct {
	Total  int `json:"total"`
	Native int `json:"native"`
	Custom int `json:"custom"`
}

type driftSummary struct {
	Total       int `json:"total"`
	Broken      int `json:"broken"`
	Missing     int `json:"missing"`
	WrongTarget int `json:"wrongTarget"`
	NotSymlink  int `json:"notSymlink"`
}

type nativeDirInfo struct {
	Dir        string `json:"dir"`
	TargetBase string `json:"targetBase"`
	Mode       string `json:"mode"`
	Exists     bool   `json:"exists"`
	FileCount  int    `json:"fileCount"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg, err := config.Load(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries, err := entry.Collect(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	drifts := drift.Detect(entries)

	// Count native vs custom
	nativeCount := 0
	for _, e := range entries {
		if !e.IsExternal && e.Line == 0 {
			nativeCount++
		}
	}

	ds := driftSummary{Total: len(drifts)}
	for _, d := range drifts {
		switch d.Type {
		case drift.DriftBroken:
			ds.Broken++
		case drift.DriftMissing:
			ds.Missing++
		case drift.DriftWrongTarget:
			ds.WrongTarget++
		case drift.DriftNotSymlink:
			ds.NotSymlink++
		}
	}

	// Native dirs info
	var ndInfos []nativeDirInfo
	for _, nd := range native.Dirs() {
		mode := "link"
		switch nd.Mode {
		case native.ModeVSCode:
			mode = "vscode"
		case native.ModeBrew:
			mode = "brew"
		case native.ModeScript:
			mode = "script"
		}

		dirPath := filepath.Join(cfg.ConfigDir, nd.Dir)
		exists := false
		fileCount := 0
		if dirEntries, err := os.ReadDir(dirPath); err == nil {
			exists = true
			for _, de := range dirEntries {
				if de.Name() != ".gitkeep" {
					fileCount++
				}
			}
		}

		ndInfos = append(ndInfos, nativeDirInfo{
			Dir:        nd.Dir,
			TargetBase: nd.TargetBase,
			Mode:       mode,
			Exists:     exists,
			FileCount:  fileCount,
		})
	}

	writeJSON(w, http.StatusOK, statusResponse{
		ConfigDir: cfg.ConfigDir,
		Version:   s.Version,
		Vars:      cfg.Vars,
		Links: linksSummary{
			Total:  len(entries),
			Native: nativeCount,
			Custom: len(entries) - nativeCount,
		},
		Drift:      ds,
		NativeDirs: ndInfos,
	})
}
