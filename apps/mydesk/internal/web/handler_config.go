package web

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/linker"
	"github.com/silee-tools/mydesk/internal/native"
)

type linksConfResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type linksConfSaveRequest struct {
	Content string `json:"content"`
	DryRun  bool   `json:"dryRun"`
}

type linksConfSaveResponse struct {
	Path          string `json:"path"`
	EntriesParsed int    `json:"entriesParsed"`
}

type nativeDirResponse struct {
	Dir        string   `json:"dir"`
	TargetBase string   `json:"targetBase"`
	Exists     bool     `json:"exists"`
	Files      []string `json:"files"`
}

type nativeDirsResponse struct {
	Dirs []nativeDirResponse `json:"dirs"`
}

func (s *Server) handleLinksConf(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	path := cfg.LinksConfPath()

	switch r.Method {
	case http.MethodGet:
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, linksConfResponse{Path: path, Content: ""})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, linksConfResponse{Path: path, Content: string(content)})

	case http.MethodPut:
		var req linksConfSaveRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Validate by writing to temp file and parsing
		tmpFile, err := os.CreateTemp("", "links-conf-*.conf")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.WriteString(req.Content); err != nil {
			tmpFile.Close()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tmpFile.Close()

		adapter := &cfgAdapter{cfg}
		entries, err := linker.ParseLinksConf(tmpPath, adapter)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if !req.DryRun {
			if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		writeJSON(w, http.StatusOK, linksConfSaveResponse{
			Path:          path,
			EntriesParsed: len(entries),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleNativeDirs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg, err := config.Load(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var dirs []nativeDirResponse
	for _, nd := range native.Dirs() {
		dirPath := filepath.Join(cfg.ConfigDir, nd.Dir)
		info := nativeDirResponse{
			Dir:        nd.Dir,
			TargetBase: nd.TargetBase,
			Files:      []string{},
		}

		dirEntries, err := os.ReadDir(dirPath)
		if err == nil {
			info.Exists = true
			for _, de := range dirEntries {
				if de.Name() != ".gitkeep" {
					info.Files = append(info.Files, de.Name())
				}
			}
		}

		dirs = append(dirs, info)
	}

	writeJSON(w, http.StatusOK, nativeDirsResponse{Dirs: dirs})
}

// cfgAdapter implements linker.ConfigDirProvider for validation.
type cfgAdapter struct {
	cfg *config.Config
}

func (a *cfgAdapter) ExpandVars(path string) string {
	return a.cfg.ExpandVars(path)
}

func (a *cfgAdapter) GetConfigDir() string {
	return a.cfg.ConfigDir
}
