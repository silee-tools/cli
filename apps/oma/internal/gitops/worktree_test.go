package gitops

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPlansRepositoryState(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	setup := "#!/bin/sh\nset -eu\n"
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "setup-worktree.sh"), []byte(setup), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore", "scripts/setup-worktree.sh")
	runGit(t, repo, "commit", "-m", "add worktree setup")
	runGit(t, repo, "push", "origin", "main")

	worktree := filepath.Join(repo, ".worktrees", "task")
	snapshot, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: worktree,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSHA := runGit(t, repo, "rev-parse", "origin/main")
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(setup)))
	wantRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RepoRoot != wantRepo || snapshot.BaseRef != "origin/main" || snapshot.BaseSHA != wantSHA || snapshot.SetupHash != wantHash {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Worktrees) != 1 || snapshot.Worktrees[0].Path != wantRepo || snapshot.Worktrees[0].Branch != "main" || snapshot.Worktrees[0].Head != wantSHA {
		t.Fatalf("unexpected worktrees: %+v", snapshot.Worktrees)
	}
}

func TestInspectRejectsDirtyCurrentWorktree(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: "current",
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("Inspect() error = %v, want dirty worktree error", err)
	}
}

func TestInspectRejectsUnignoredAndPartialWorktreeStates(t *testing.T) {
	t.Run("unignored", func(t *testing.T) {
		repo, _ := newRemoteRepo(t)
		_, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{
			Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(repo, ".worktrees", "task"),
		})
		if err == nil || !strings.Contains(err.Error(), "ignore") {
			t.Fatalf("Inspect() error = %v, want ignore error", err)
		}
	})

	t.Run("path-only", func(t *testing.T) {
		repo, _ := newRemoteRepo(t)
		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		worktree := filepath.Join(repo, ".worktrees", "task")
		if err := os.MkdirAll(worktree, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{
			Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: worktree,
		})
		if err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("Inspect() error = %v, want ordinary-directory conflict", err)
		}
	})
}

func TestInspectReusesExactWorktreeAndRejectsPartialConflicts(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore worktrees")
	runGit(t, repo, "push", "origin", "main")

	exact := filepath.Join(repo, ".worktrees", "task")
	sha := runGit(t, repo, "rev-parse", "origin/main")
	if err := CreateWorktree(context.Background(), CommandRunner{}, Operation{Repo: repo, Path: exact, Branch: "feature/task", BaseSHA: sha}); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: exact}); err != nil {
		t.Fatalf("Inspect() rejected exact reusable state: %v", err)
	}
	commitFile(t, repo, "new-base.txt", "new base\n", "advance base")
	runGit(t, repo, "push", "origin", "main")
	_, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: exact})
	if err == nil || !strings.Contains(err.Error(), "base") {
		t.Fatalf("Inspect() error = %v, want reusable worktree base mismatch", err)
	}

	_, err = Inspect(context.Background(), CommandRunner{}, InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/other", Worktree: exact})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("Inspect() error = %v, want path/branch conflict", err)
	}

	otherPath := filepath.Join(repo, ".worktrees", "other")
	_, err = Inspect(context.Background(), CommandRunner{}, InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: otherPath})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("Inspect() error = %v, want branch/path conflict", err)
	}
}

func TestInspectSelectedSubmoduleUsesIndependentRemoteHead(t *testing.T) {
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	parent, _ := newRemoteRepo(t)
	first, _ := newRemoteRepo(t)
	second, _ := newRemoteRepo(t)
	runGit(t, first, "branch", "-m", "trunk")
	runGit(t, first, "push", "origin", "trunk")
	runGit(t, filepath.Join(filepath.Dir(first), "origin.git"), "symbolic-ref", "HEAD", "refs/heads/trunk")
	runGit(t, first, "push", "origin", ":main")
	firstSHA := commitFile(t, first, "independent.txt", "first\n", "independent base")
	runGit(t, first, "push", "origin", "trunk")

	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", filepath.Join(filepath.Dir(first), "origin.git"), "modules/first")
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", filepath.Join(filepath.Dir(second), "origin.git"), "modules/second")
	runGit(t, parent, "commit", "-m", "add submodules")
	runGit(t, parent, "push", "origin", "main")
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitignore")
	runGit(t, parent, "commit", "-m", "ignore worktrees")
	runGit(t, parent, "push", "origin", "main")

	snapshot, err := Inspect(context.Background(), CommandRunner{}, InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(parent, ".worktrees", "task"),
		Submodules: []string{"modules/first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Submodules) != 1 || snapshot.Submodules[0].Path != "modules/first" || snapshot.Submodules[0].BaseRef != "trunk" || snapshot.Submodules[0].BaseSHA != firstSHA {
		t.Fatalf("unexpected submodule plan: %+v", snapshot.Submodules)
	}
}

func TestPrepareSubmodulesInitializesOnlySelectedPath(t *testing.T) {
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	parent, _ := newRemoteRepo(t)
	first, _ := newRemoteRepo(t)
	second, _ := newRemoteRepo(t)
	firstOrigin := filepath.Join(filepath.Dir(first), "origin.git")
	secondOrigin := filepath.Join(filepath.Dir(second), "origin.git")
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", firstOrigin, "modules/first")
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", secondOrigin, "modules/second")
	runGit(t, parent, "commit", "-m", "add submodules")
	base := runGit(t, parent, "rev-parse", "HEAD")
	worktree := filepath.Join(t.TempDir(), "worktree")
	if err := CreateWorktree(context.Background(), CommandRunner{}, Operation{Repo: parent, Path: worktree, Branch: "feature/task", BaseSHA: base}); err != nil {
		t.Fatal(err)
	}

	firstSHA := runGit(t, first, "rev-parse", "HEAD")
	operations := []SubmoduleOperation{{Path: "modules/first", URL: firstOrigin, Branch: "feature/task", BaseRef: "main", BaseSHA: firstSHA}}
	if err := PrepareSubmodules(context.Background(), CommandRunner{}, worktree, operations); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, filepath.Join(worktree, "modules", "first"), "branch", "--show-current"); got != "feature/task" {
		t.Fatalf("selected submodule branch = %q, want feature/task", got)
	}
	if got := runGit(t, filepath.Join(worktree, "modules", "first"), "rev-parse", "HEAD"); got != firstSHA {
		t.Fatalf("selected submodule HEAD = %q, want %q", got, firstSHA)
	}
	if _, err := os.Stat(filepath.Join(worktree, "modules", "second", ".git")); !os.IsNotExist(err) {
		t.Fatalf("unselected submodule was initialized: %v", err)
	}
}

func TestRunSetup(t *testing.T) {
	t.Run("missing script is a no-op", func(t *testing.T) {
		if err := RunSetup(context.Background(), t.TempDir(), nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("uses target cwd and distinct argv", func(t *testing.T) {
		worktree := t.TempDir()
		if err := os.MkdirAll(filepath.Join(worktree, "scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\nset -eu\npwd > setup.cwd\nprintf '%s\\n' \"$@\" > setup.args\n"
		if err := os.WriteFile(filepath.Join(worktree, "scripts", "setup-worktree.sh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		args := []string{"two words", "$(touch should-not-exist)", "--flag=value"}
		if err := RunSetup(context.Background(), worktree, args); err != nil {
			t.Fatal(err)
		}
		cwd, err := os.ReadFile(filepath.Join(worktree, "setup.cwd"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(cwd)) != worktree {
			t.Fatalf("setup cwd = %q, want %q", strings.TrimSpace(string(cwd)), worktree)
		}
		gotArgs, err := os.ReadFile(filepath.Join(worktree, "setup.args"))
		if err != nil {
			t.Fatal(err)
		}
		if string(gotArgs) != strings.Join(args, "\n")+"\n" {
			t.Fatalf("setup args = %q, want %q", gotArgs, strings.Join(args, "\n")+"\n")
		}
		if _, err := os.Stat(filepath.Join(worktree, "should-not-exist")); !os.IsNotExist(err) {
			t.Fatalf("setup argument was evaluated as shell code: %v", err)
		}
	})

	t.Run("failure is returned without removing worktree", func(t *testing.T) {
		worktree := t.TempDir()
		if err := os.MkdirAll(filepath.Join(worktree, "scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\nset -eu\n: \"${1:?setup argument required}\"\n"
		if err := os.WriteFile(filepath.Join(worktree, "scripts", "setup-worktree.sh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := RunSetup(context.Background(), worktree, nil); err == nil {
			t.Fatal("RunSetup() succeeded without required setup argument")
		}
		if info, err := os.Stat(worktree); err != nil || !info.IsDir() {
			t.Fatalf("worktree was removed after setup failure: %v", err)
		}
	})
}
