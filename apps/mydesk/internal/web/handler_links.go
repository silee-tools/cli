package web

import (
	"net/http"
	"os"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/entry"
	"github.com/silee-tools/mydesk/internal/linker"
)

type linkEntryResponse struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	SrcAbs     string `json:"srcAbs"`
	DstAbs     string `json:"dstAbs"`
	IsExternal bool   `json:"isExternal"`
	Status     string `json:"status"`
}

type linksResponse struct {
	Entries []linkEntryResponse `json:"entries"`
	Summary struct {
		Total   int `json:"total"`
		Linked  int `json:"linked"`
		Drifted int `json:"drifted"`
	} `json:"summary"`
}

type actionRequest struct {
	DryRun bool `json:"dryRun"`
}

type reportResponse struct {
	Report struct {
		Linked   int      `json:"linked"`
		Skipped  int      `json:"skipped"`
		BackedUp int      `json:"backedUp"`
		Failed   int      `json:"failed"`
		Errors   []string `json:"errors"`
	} `json:"report"`
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
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

	resp := linksResponse{}
	linked := 0
	drifted := 0

	for _, e := range entries {
		status := checkLinkStatus(e)
		if status == "linked" {
			linked++
		} else {
			drifted++
		}
		resp.Entries = append(resp.Entries, linkEntryResponse{
			Source:     e.Source,
			Target:     e.Target,
			SrcAbs:     e.SrcAbs,
			DstAbs:     e.DstAbs,
			IsExternal: e.IsExternal,
			Status:     status,
		})
	}

	resp.Summary.Total = len(entries)
	resp.Summary.Linked = linked
	resp.Summary.Drifted = drifted

	writeJSON(w, http.StatusOK, resp)
}

func checkLinkStatus(e linker.LinkEntry) string {
	info, err := os.Lstat(e.DstAbs)
	if err != nil {
		return "missing"
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "not_symlink"
	}
	target, err := os.Readlink(e.DstAbs)
	if err != nil {
		return "broken"
	}
	if target != e.SrcAbs {
		return "wrong_target"
	}
	if _, err := os.Stat(e.SrcAbs); err != nil {
		return "broken"
	}
	return "linked"
}

func (s *Server) handleLinkAction(w http.ResponseWriter, r *http.Request) {
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

	entries, err := entry.Collect(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	report := linker.New(entries, req.DryRun).Link()
	writeJSON(w, http.StatusOK, toReportResponse(report))
}

func (s *Server) handleUnlinkAction(w http.ResponseWriter, r *http.Request) {
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

	entries, err := entry.Collect(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	report := linker.New(entries, req.DryRun).Unlink()
	writeJSON(w, http.StatusOK, toReportResponse(report))
}

func toReportResponse(r linker.LinkReport) reportResponse {
	resp := reportResponse{}
	resp.Report.Linked = r.Linked
	resp.Report.Skipped = r.Skipped
	resp.Report.BackedUp = r.BackedUp
	resp.Report.Failed = r.Failed
	for _, e := range r.Errors {
		resp.Report.Errors = append(resp.Report.Errors, e.Error())
	}
	if resp.Report.Errors == nil {
		resp.Report.Errors = []string{}
	}
	return resp
}
