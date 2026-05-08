package web

import (
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/provision"
)

type provisionModuleStatus struct {
	Available      bool   `json:"available"`
	Path           string `json:"path,omitempty"`
	ExtensionCount int    `json:"extensionCount,omitempty"`
}

type provisionStatusResponse struct {
	Brew   provisionModuleStatus `json:"brew"`
	VSCode provisionModuleStatus `json:"vscode"`
	OMZ    provisionModuleStatus `json:"omz"`
	MacOS  provisionModuleStatus `json:"macos"`
	Mise   provisionModuleStatus `json:"mise"`
}

type provisionActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) handleProvisionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg, err := config.Load(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := provisionStatusResponse{}

	// Brew
	brewPath := filepath.Join(cfg.ConfigDir, "brew", "Brewfile")
	resp.Brew.Path = brewPath
	resp.Brew.Available = fileExists(brewPath)

	// VS Code
	vscodePath := filepath.Join(cfg.ConfigDir, "vscode", "extensions.txt")
	resp.VSCode.Path = vscodePath
	resp.VSCode.Available = fileExists(vscodePath)
	if resp.VSCode.Available {
		if data, err := os.ReadFile(vscodePath); err == nil {
			count := 0
			for _, line := range strings.Split(string(data), "\n") {
				if strings.TrimSpace(line) != "" {
					count++
				}
			}
			resp.VSCode.ExtensionCount = count
		}
	}

	// OMZ
	omzPath := filepath.Join(cfg.ConfigDir, "omz", "install.sh")
	resp.OMZ.Path = omzPath
	resp.OMZ.Available = fileExists(omzPath)

	// macOS
	macosPath := filepath.Join(cfg.ConfigDir, "macos", "defaults.sh")
	resp.MacOS.Path = macosPath
	resp.MacOS.Available = fileExists(macosPath)

	// mise
	resp.Mise.Available = commandExists("mise")

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleProvisionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract action from path: /api/provision/{action}
	action := strings.TrimPrefix(r.URL.Path, "/api/provision/")
	allowed := map[string]bool{
		"brew-sync": true, "brew-install": true,
		"vscode-sync": true, "vscode-install": true,
		"omz-install": true, "mise-install": true, "apply-defaults": true,
	}
	if !allowed[action] {
		writeError(w, http.StatusBadRequest, "unknown action: "+action)
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

	var actionErr error
	var msg string

	switch action {
	case "brew-sync":
		actionErr = provision.BrewSync(cfg.ConfigDir, runner)
		msg = "Brewfile updated"
	case "brew-install":
		actionErr = provision.BrewInstall(cfg.ConfigDir, runner)
		msg = "Homebrew packages installed"
	case "vscode-sync":
		actionErr = provision.VSCodeSync(cfg.ConfigDir, runner)
		msg = "extensions.txt updated"
	case "vscode-install":
		actionErr = provision.VSCodeInstall(cfg.ConfigDir, runner)
		msg = "VS Code extensions installed"
	case "omz-install":
		actionErr = provision.OMZInstall(cfg.ConfigDir, runner)
		msg = "Oh-My-Zsh setup complete"
	case "mise-install":
		actionErr = provision.MiseInstall(runner)
		msg = "mise runtimes installed"
	case "apply-defaults":
		actionErr = provision.ApplyDefaults(cfg.ConfigDir, runner)
		msg = "macOS defaults applied"
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+action)
		return
	}

	if actionErr != nil {
		writeError(w, http.StatusInternalServerError, actionErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, provisionActionResponse{Success: true, Message: msg})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := osexec.LookPath(name)
	return err == nil
}
