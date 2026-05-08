package exec

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/silee-tools/mydesk/internal/ui"
)

type Runner struct {
	DryRun  bool
	Verbose bool
}

func New(dryRun, verbose bool) *Runner {
	return &Runner{DryRun: dryRun, Verbose: verbose}
}

func (r *Runner) Run(name string, args ...string) error {
	if r.DryRun {
		ui.DryRun("exec: %s %s", name, strings.Join(args, " "))
		return nil
	}
	if r.Verbose {
		ui.Info("exec: %s %s", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Runner) RunOutput(name string, args ...string) (string, error) {
	if r.DryRun {
		ui.DryRun("exec: %s %s", name, strings.Join(args, " "))
		return "", nil
	}
	if r.Verbose {
		ui.Info("exec: %s %s", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}

func (r *Runner) RunScript(path string) error {
	if r.DryRun {
		ui.DryRun("exec: bash -e %s", path)
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("script not found: %s", path)
	}
	return r.Run("bash", "-e", path)
}
