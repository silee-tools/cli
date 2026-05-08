package web

import (
	"net/http"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/drift"
	"github.com/silee-tools/mydesk/internal/entry"
)

type driftResultResponse struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

type driftResponse struct {
	Results      []driftResultResponse `json:"results"`
	TotalEntries int                   `json:"totalEntries"`
	DriftCount   int                   `json:"driftCount"`
}

func (s *Server) handleDrift(w http.ResponseWriter, r *http.Request) {
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

	results := make([]driftResultResponse, 0, len(drifts))
	for _, d := range drifts {
		typeName := "unknown"
		switch d.Type {
		case drift.DriftBroken:
			typeName = "broken"
		case drift.DriftMissing:
			typeName = "missing"
		case drift.DriftWrongTarget:
			typeName = "wrong_target"
		case drift.DriftNotSymlink:
			typeName = "not_symlink"
		}
		results = append(results, driftResultResponse{
			Source: d.Entry.Source,
			Target: d.Entry.Target,
			Type:   typeName,
			Detail: d.Detail,
		})
	}

	writeJSON(w, http.StatusOK, driftResponse{
		Results:      results,
		TotalEntries: len(entries),
		DriftCount:   len(drifts),
	})
}
