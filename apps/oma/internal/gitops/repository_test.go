package gitops

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeRepoAndDefaultBase(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	nested := filepath.Join(repo, "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{inner: testRunner(t)}
	root, common, err := NormalizeRepo(context.Background(), runner, nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.EvalSymlinks(repo)
	wantCommon, _ := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if root != wantRoot || common != wantCommon {
		t.Fatalf("NormalizeRepo() = (%q, %q), want (%q, %q)", root, common, wantRoot, wantCommon)
	}

	base, candidates, err := DefaultBase(context.Background(), runner, root)
	if err != nil {
		t.Fatal(err)
	}
	if base != "origin/main" {
		t.Fatalf("DefaultBase() = %q, want origin/main", base)
	}
	if len(candidates) == 0 || candidates[0] != "origin/main" {
		t.Fatalf("candidates = %q, want deterministic origin/main first", candidates)
	}

	runner.mu.Lock()
	commands := append([]recordedCommand(nil), runner.log...)
	runner.mu.Unlock()
	for _, command := range commands {
		if command.name == "git" {
			assertCommandUsesRepo(t, command, command.args[1])
		}
	}
}

func TestNormalizeRepoRejectsNonRepository(t *testing.T) {
	root, common, err := NormalizeRepo(context.Background(), testRunner(t), t.TempDir())
	if err == nil || root != "" || common != "" {
		t.Fatalf("NormalizeRepo() = (%q, %q, %v), want non-empty error and empty paths", root, common, err)
	}
}

func TestFetchOriginDoesNotPrune(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	runner := &recordingRunner{inner: testRunner(t)}
	if err := FetchOrigin(context.Background(), runner, repo); err != nil {
		t.Fatal(err)
	}
	if runner.containsSequence("fetch", "--prune") || runner.containsSequence("fetch", "-p") {
		t.Fatalf("FetchOrigin used a pruning fetch:\n%s", commandDump(runner.log))
	}
	if !runner.containsSequence("fetch", "origin") {
		t.Fatalf("FetchOrigin did not fetch origin:\n%s", commandDump(runner.log))
	}
}

func TestFetchOriginWithoutRemoteIsNoOp(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	runGit(t, "", "init", "--initial-branch=main", repo)
	configureIdentity(t, repo)
	commitFile(t, repo, "README.md", "local\n", "initial")
	if err := FetchOrigin(context.Background(), testRunner(t), repo); err != nil {
		t.Fatalf("FetchOrigin() without origin returned %v", err)
	}
}

func TestDefaultBaseFallbackOrder(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	runGit(t, "", "init", "--initial-branch=master", repo)
	configureIdentity(t, repo)
	commitFile(t, repo, "README.md", "local\n", "initial")
	runGit(t, repo, "branch", "main")
	runGit(t, repo, "branch", "hotfix/z-last")
	runGit(t, repo, "branch", "change/a-first")
	runGit(t, repo, "branch", "release/2")

	base, candidates, err := DefaultBase(context.Background(), testRunner(t), repo)
	if err != nil {
		t.Fatal(err)
	}
	if base != "main" {
		t.Fatalf("DefaultBase() = %q, want main", base)
	}
	want := []string{"main", "master", "hotfix/z-last", "change/a-first", "release/2"}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("candidates = %q, want %q", candidates, want)
	}
}

func TestNormalizeRepoFromLinkedWorktree(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-b", "feature/linked", worktree, "main")

	root, common, err := NormalizeRepo(context.Background(), testRunner(t), worktree)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.EvalSymlinks(worktree)
	wantCommon, _ := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if root != wantRoot || common != wantCommon || !filepath.IsAbs(root) || !filepath.IsAbs(common) {
		t.Fatalf("NormalizeRepo() = (%q, %q), want absolute (%q, %q)", root, common, wantRoot, wantCommon)
	}
}

func TestDefaultBaseReportsMissingHead(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "empty")
	runGit(t, "", "init", repo)
	base, candidates, err := DefaultBase(context.Background(), testRunner(t), repo)
	if err == nil || base != "" || len(candidates) != 0 || !strings.Contains(err.Error(), "base") {
		t.Fatalf("DefaultBase() = (%q, %q, %v), want descriptive base error", base, candidates, err)
	}
}
