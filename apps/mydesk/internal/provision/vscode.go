package provision

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/silee-tools/mydesk/internal/exec"
	"github.com/silee-tools/mydesk/internal/ui"
)

func VSCodeSync(configDir string, runner *exec.Runner) error {
	extFile := filepath.Join(configDir, "vscode", "extensions.txt")

	if err := os.MkdirAll(filepath.Dir(extFile), 0755); err != nil {
		return err
	}

	output, err := runner.RunOutput("code", "--list-extensions")
	if err != nil {
		return err
	}

	return os.WriteFile(extFile, []byte(output+"\n"), 0644)
}

func VSCodeInstall(configDir string, runner *exec.Runner) error {
	extFile := filepath.Join(configDir, "vscode", "extensions.txt")

	f, err := os.Open(extFile)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Warning("vscode/extensions.txt not found, skipping")
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ext := strings.TrimSpace(scanner.Text())
		if ext == "" {
			continue
		}
		if err := runner.Run("code", "--install-extension", ext); err != nil {
			ui.Warning("failed to install VS Code extension: %s", ext)
		}
	}

	return scanner.Err()
}
