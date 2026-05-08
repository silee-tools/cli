package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/silee-tools/jg/internal/entry"
)

// Run launches fzf with the given entries and optional query.
// Returns the selected path or empty string if cancelled.
func Run(entries []entry.Entry, query string) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("fzf not found. Install it: brew install fzf")
	}

	home, _ := os.UserHomeDir()

	args := []string{
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--select-1",
		"--keep-right",
		"--wrap",
		"--header=Git Repos",
		"--preview", previewCmd(home),
	}
	if query != "" {
		args = append(args, "--query", shortenPath(query, home))
	}

	cmd := exec.Command(fzfPath, args...)
	cmd.Stderr = os.Stderr

	var input strings.Builder
	for _, e := range entries {
		fmt.Fprintln(&input, shortenPath(e.Path, home))
	}
	cmd.Stdin = strings.NewReader(input.String())

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf exit 1 = no match, exit 130 = cancelled
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", err
	}

	selected := strings.TrimSpace(string(out))
	return expandPath(selected, home), nil
}

// shortenPath replaces $HOME prefix with ~ for compact display.
func shortenPath(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(os.PathSeparator))) {
		return "~" + path[len(home):]
	}
	return path
}

// expandPath restores ~ back to the absolute home directory path.
func expandPath(path, home string) string {
	if home != "" && strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

// previewCmd builds the fzf preview command, expanding ~ to $HOME for git commands.
func previewCmd(home string) string {
	// Use shell variable expansion: replace ~ with actual home in the path before passing to git
	resolve := fmt.Sprintf(`p="{}"; p="${p/#\\~/%s}"`, home)
	return resolve + `; git -C "$p" log --oneline -5 2>/dev/null; echo; echo "branch: $(git -C "$p" branch --show-current 2>/dev/null)"; echo; git -C "$p" status --short 2>/dev/null | head -10`
}
