package web

import (
	"net/http"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/provision"
)

type syncResult struct {
	Task    string `json:"task"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type syncResponse struct {
	Results []syncResult `json:"results"`
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req actionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := config.Load(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	runner := exec.New(req.DryRun, false)
	var results []syncResult

	if err := provision.BrewSync(cfg.ConfigDir, runner); err != nil {
		results = append(results, syncResult{Task: "brew", Success: false, Message: err.Error()})
	} else {
		results = append(results, syncResult{Task: "brew", Success: true, Message: "Brewfile updated"})
	}

	if err := provision.VSCodeSync(cfg.ConfigDir, runner); err != nil {
		results = append(results, syncResult{Task: "vscode", Success: false, Message: err.Error()})
	} else {
		results = append(results, syncResult{Task: "vscode", Success: true, Message: "extensions.txt updated"})
	}

	writeJSON(w, http.StatusOK, syncResponse{Results: results})
}
