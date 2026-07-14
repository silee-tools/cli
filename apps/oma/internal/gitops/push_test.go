package gitops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktreeAndPush(t *testing.T) {
	repo, origin := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore worktrees")
	runGit(t, repo, "push", "origin", "main")
	sha := runGit(t, repo, "rev-parse", "HEAD")
	worktree := filepath.Join(repo, ".worktrees", "task")
	runner := &recordingRunner{inner: CommandRunner{}}

	operation := Operation{Repo: repo, Path: worktree, Branch: "feature/task", BaseSHA: sha}
	if err := CreateWorktree(context.Background(), runner, operation); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, worktree, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("worktree HEAD = %q, want %q", got, sha)
	}
	if got := runGit(t, worktree, "branch", "--show-current"); got != "feature/task" {
		t.Fatalf("worktree branch = %q, want feature/task", got)
	}
	if err := CreateWorktree(context.Background(), runner, operation); err != nil {
		t.Fatalf("CreateWorktree() did not reuse exact state: %v", err)
	}
	if err := Push(context.Background(), runner, worktree, "feature/task"); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, origin, "rev-parse", "refs/heads/feature/task"); got != sha {
		t.Fatalf("remote branch = %q, want %q", got, sha)
	}
	if err := Push(context.Background(), runner, worktree, "feature/task"); err != nil {
		t.Fatalf("Push() did not reuse identical remote SHA: %v", err)
	}
	if runner.containsSequence("push", "--force") || runner.containsSequence("push", "-f") || runner.containsSequence("push", "--force-with-lease") {
		t.Fatalf("dangerous force push found:\n%s", commandDump(runner.log))
	}
	if !runner.containsSequence("push", "-u", "origin", "feature/task") {
		t.Fatalf("normal upstream push not found:\n%s", commandDump(runner.log))
	}
}

func TestCreateWorktreeValidatesBranchBeforeMutation(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	path := filepath.Join(t.TempDir(), "worktree")
	err := CreateWorktree(context.Background(), CommandRunner{}, Operation{
		Repo: repo, Path: path, Branch: "bad branch", BaseSHA: runGit(t, repo, "rev-parse", "HEAD"),
	})
	if err == nil || !strings.Contains(err.Error(), "branch") {
		t.Fatalf("CreateWorktree() error = %v, want branch validation error", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid branch mutated worktree path: %v", statErr)
	}
}

func TestPushRejectsMissingRemoteAndRemoteChanges(t *testing.T) {
	t.Run("missing origin", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "repo")
		runGit(t, "", "init", "--initial-branch=main", repo)
		configureIdentity(t, repo)
		commitFile(t, repo, "README.md", "local\n", "initial")
		if err := Push(context.Background(), CommandRunner{}, repo, "main"); err == nil || !strings.Contains(err.Error(), "origin") {
			t.Fatalf("Push() error = %v, want missing origin error", err)
		}
	})

	for _, test := range []struct {
		name      string
		remoteRef string
	}{
		{name: "remote ahead", remoteRef: "HEAD"},
		{name: "remote diverged", remoteRef: "HEAD~1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, origin := newRemoteRepo(t)
			branch := "feature/task"
			runGit(t, repo, "checkout", "-b", branch)
			localSHA := commitFile(t, repo, "local.txt", "local\n", "local")
			runGit(t, repo, "push", "-u", "origin", branch)

			other := filepath.Join(t.TempDir(), "other")
			runGit(t, "", "clone", origin, other)
			configureIdentity(t, other)
			runGit(t, other, "checkout", branch)
			if test.name == "remote diverged" {
				runGit(t, other, "reset", "--hard", test.remoteRef)
			}
			commitFile(t, other, "remote.txt", test.name+"\n", "remote change")
			forceArgs := []string{"push", "origin", branch}
			if test.name == "remote diverged" {
				forceArgs = []string{"push", "--force", "origin", branch}
			}
			runGit(t, other, forceArgs...)

			runner := &recordingRunner{inner: CommandRunner{}}
			err := Push(context.Background(), runner, repo, branch)
			if err == nil || !strings.Contains(err.Error(), "remote") {
				t.Fatalf("Push() error = %v, want remote mismatch error", err)
			}
			if got := runGit(t, repo, "rev-parse", branch); got != localSHA {
				t.Fatalf("Push() rewrote local branch: got %q, want %q", got, localSHA)
			}
			if runner.containsSequence("push", "--force") || runner.containsSequence("push", "-f") || runner.containsSequence("push", "--force-with-lease") {
				t.Fatalf("Push() attempted force:\n%s", commandDump(runner.log))
			}
		})
	}
}
