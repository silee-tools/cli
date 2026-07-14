package gitops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	inner Runner
	mu    sync.Mutex
	log   []recordedCommand
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.log = append(r.log, recordedCommand{name: name, args: append([]string(nil), args...)})
	r.mu.Unlock()
	return r.inner.Run(ctx, name, args...)
}

func (r *recordingRunner) containsSequence(want ...string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, command := range r.log {
		all := append([]string{command.name}, command.args...)
		for i := 0; i+len(want) <= len(all); i++ {
			if strings.Join(all[i:i+len(want)], "\x00") == strings.Join(want, "\x00") {
				return true
			}
		}
	}
	return false
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	commandArgs := append([]string(nil), args...)
	if dir != "" {
		commandArgs = append([]string{"-C", dir}, commandArgs...)
	}
	cmd := exec.Command("git", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q failed: %v\n%s", commandArgs, err, output)
	}
	return strings.TrimSpace(string(output))
}

func configureIdentity(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "config", "user.name", "Oma Test")
	runGit(t, repo, "config", "user.email", "oma@example.com")
	runGit(t, repo, "config", "core.excludesFile", "/dev/null")
}

func newRemoteRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	runGit(t, "", "init", "--bare", "--initial-branch=main", origin)
	runGit(t, "", "init", "--initial-branch=main", repo)
	configureIdentity(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("oma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	runGit(t, repo, "remote", "add", "origin", origin)
	runGit(t, repo, "push", "-u", "origin", "main")
	runGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	return repo, origin
}

func commitFile(t *testing.T, repo, name, contents, message string) string {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "--", name)
	runGit(t, repo, "commit", "-m", message)
	return runGit(t, repo, "rev-parse", "HEAD")
}

func assertCommandUsesRepo(t *testing.T, command recordedCommand, repo string) {
	t.Helper()
	if command.name != "git" || len(command.args) < 2 || command.args[0] != "-C" || command.args[1] != repo {
		t.Fatalf("command does not use an explicit repository: %s %q", command.name, command.args)
	}
}

func commandDump(commands []recordedCommand) string {
	var lines []string
	for _, command := range commands {
		lines = append(lines, fmt.Sprintf("%s %q", command.name, command.args))
	}
	return strings.Join(lines, "\n")
}
