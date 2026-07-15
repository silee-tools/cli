package gitops

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
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
	if err := os.WriteFile(filepath.Join(repo, "scripts", "setup-worktree.sh"), []byte("working checkout differs\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	worktree := filepath.Join(repo, ".worktrees", "task")
	snapshot, err := Inspect(context.Background(), testRunner(t), InspectRequest{
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

func TestInspectSetupHashIsEmptyWhenScriptIsMissingFromBase(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore worktrees")
	runGit(t, repo, "push", "origin", "main")
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "setup-worktree.sh"), []byte("working checkout only\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(repo, ".worktrees", "task"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SetupHash != "" {
		t.Fatalf("SetupHash = %q, want empty for script missing from selected base", snapshot.SetupHash)
	}
}

func TestInspectRejectsSetupSymlinkInSelectedBase(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "external-setup.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../external-setup.sh", filepath.Join(repo, "scripts", "setup-worktree.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore", "external-setup.sh", "scripts/setup-worktree.sh")
	runGit(t, repo, "commit", "-m", "add setup symlink")
	runGit(t, repo, "push", "origin", "main")

	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(repo, ".worktrees", "task"),
	})
	if err == nil || !strings.Contains(err.Error(), "regular blob") {
		t.Fatalf("Inspect() error = %v, want setup symlink rejection", err)
	}
}

func TestInspectRejectsDirtyCurrentWorktree(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: "current",
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("Inspect() error = %v, want dirty worktree error", err)
	}
}

func TestInspectRejectsUnignoredAndPartialWorktreeStates(t *testing.T) {
	t.Run("unignored", func(t *testing.T) {
		repo, _ := newRemoteRepo(t)
		_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
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
		_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
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
	if err := CreateWorktree(context.Background(), testRunner(t), Operation{Repo: repo, Path: exact, Branch: "feature/task", BaseSHA: sha}); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), testRunner(t), InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: exact}); err != nil {
		t.Fatalf("Inspect() rejected exact reusable state: %v", err)
	}
	commitFile(t, repo, "new-base.txt", "new base\n", "advance base")
	runGit(t, repo, "push", "origin", "main")
	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: exact})
	if err == nil || !strings.Contains(err.Error(), "base") {
		t.Fatalf("Inspect() error = %v, want reusable worktree base mismatch", err)
	}

	_, err = Inspect(context.Background(), testRunner(t), InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/other", Worktree: exact})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("Inspect() error = %v, want path/branch conflict", err)
	}

	otherPath := filepath.Join(repo, ".worktrees", "other")
	_, err = Inspect(context.Background(), testRunner(t), InspectRequest{Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: otherPath})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("Inspect() error = %v, want branch/path conflict", err)
	}
}

func TestInspectHandlesPrunableWorktreeRecordsWithoutPruning(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore worktrees")
	runGit(t, repo, "push", "origin", "main")
	stalePath := filepath.Join(repo, ".worktrees", "stale")
	runGit(t, repo, "worktree", "add", "-b", "feature/stale", stalePath, "main")
	if err := os.RemoveAll(stalePath); err != nil {
		t.Fatal(err)
	}
	before := runGit(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(before, "prunable") {
		t.Fatalf("fixture is not prunable:\n%s", before)
	}

	if _, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(repo, ".worktrees", "task"),
	}); err != nil {
		t.Fatalf("unrelated prunable worktree blocked inspection: %v", err)
	}
	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/stale", Worktree: filepath.Join(repo, ".worktrees", "replacement"),
	})
	if err == nil || !strings.Contains(err.Error(), "prunable") {
		t.Fatalf("Inspect() error = %v, want same-branch prunable conflict", err)
	}
	_, err = Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "origin/main", Branch: "feature/stale", Worktree: stalePath,
	})
	if err == nil || !strings.Contains(err.Error(), "prunable") {
		t.Fatalf("Inspect() error = %v, want deterministic same-path prunable conflict", err)
	}
	after := runGit(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(after, "prunable") {
		t.Fatalf("Inspect() pruned stale worktree unexpectedly:\n%s", after)
	}
}

func TestListWorktreeRecordsPreservesPorcelainStates(t *testing.T) {
	repo, _ := newRemoteRepo(t)
	detached := filepath.Join(t.TempDir(), "detached")
	runGit(t, repo, "worktree", "add", "--detach", detached, "main")
	runGit(t, repo, "worktree", "lock", detached)
	records, err := listWorktreeRecords(context.Background(), testRunner(t), repo)
	if err != nil {
		t.Fatal(err)
	}
	wantDetached, err := canonicalTarget(detached)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.Path == wantDetached {
			found = true
			if !record.Detached || !record.Locked || record.Bare || record.Prunable {
				t.Fatalf("detached locked record lost porcelain state: %+v", record)
			}
		}
	}
	if !found {
		t.Fatalf("detached worktree missing from records: %+v", records)
	}

	bare := filepath.Join(t.TempDir(), "bare.git")
	runGit(t, "", "init", "--bare", bare)
	bareRecords, err := listWorktreeRecords(context.Background(), testRunner(t), bare)
	if err != nil {
		t.Fatal(err)
	}
	if len(bareRecords) != 1 || !bareRecords[0].Bare {
		t.Fatalf("bare worktree record lost porcelain state: %+v", bareRecords)
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

	snapshot, err := Inspect(context.Background(), testRunner(t), InspectRequest{
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

func TestInspectResolvesRelativeSubmoduleURLFromSuperprojectOrigin(t *testing.T) {
	root := t.TempDir()
	remotes := filepath.Join(root, "remotes")
	if err := os.MkdirAll(remotes, 0o755); err != nil {
		t.Fatal(err)
	}
	childOrigin := filepath.Join(remotes, "child.git")
	childWork := filepath.Join(root, "child-work")
	runGit(t, "", "init", "--bare", "--initial-branch=main", childOrigin)
	runGit(t, "", "init", "--initial-branch=main", childWork)
	configureIdentity(t, childWork)
	childSHA := commitFile(t, childWork, "child.txt", "child\n", "child initial")
	runGit(t, childWork, "remote", "add", "origin", childOrigin)
	runGit(t, childWork, "push", "-u", "origin", "main")

	parentOrigin := filepath.Join(remotes, "parent.git")
	parent := filepath.Join(root, "parent-work")
	runGit(t, "", "init", "--bare", "--initial-branch=main", parentOrigin)
	runGit(t, "", "init", "--initial-branch=main", parent)
	configureIdentity(t, parent)
	modules := "[submodule \"child\"]\n\tpath = modules/child\n\turl = ../child.git\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(modules), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitmodules", ".gitignore")
	runGit(t, parent, "commit", "-m", "add relative submodule")
	runGit(t, parent, "remote", "add", "origin", parentOrigin)
	runGit(t, parent, "push", "-u", "origin", "main")

	snapshot, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task",
		Worktree: filepath.Join(parent, ".worktrees", "task"), Submodules: []string{"modules/child"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Submodules) != 1 || snapshot.Submodules[0].URL != "../child.git" || snapshot.Submodules[0].BaseRef != "main" || snapshot.Submodules[0].BaseSHA != childSHA {
		t.Fatalf("unexpected relative submodule plan: %+v", snapshot.Submodules)
	}
}

func TestInspectReadsSubmodulesFromSelectedBase(t *testing.T) {
	parent, _ := newRemoteRepo(t)
	_, baseOrigin := newRemoteRepo(t)
	baseSHA := runGit(t, baseOrigin, "rev-parse", "HEAD")
	_, currentOrigin := newRemoteRepo(t)
	baseModules := "[submodule \"base\"]\n\tpath = modules/base\n\turl = " + baseOrigin + "\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(baseModules), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitmodules", ".gitignore")
	runGit(t, parent, "commit", "-m", "add base submodule declaration")
	runGit(t, parent, "push", "origin", "main")
	currentModules := "[submodule \"current\"]\n\tpath = modules/current\n\turl = " + currentOrigin + "\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(currentModules), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{inner: testRunner(t)}
	snapshot, err := Inspect(context.Background(), runner, InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task",
		Worktree: filepath.Join(parent, ".worktrees", "task"), Submodules: []string{"modules/base"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Submodules) != 1 || snapshot.Submodules[0].Path != "modules/base" || snapshot.Submodules[0].URL != baseOrigin || snapshot.Submodules[0].BaseSHA != baseSHA {
		t.Fatalf("Inspect used current .gitmodules instead of selected base: %+v", snapshot.Submodules)
	}
	if runner.containsSequence("ls-remote", "--symref", "--", currentOrigin, "HEAD") {
		t.Fatalf("Inspect contacted current-checkout submodule remote:\n%s", commandDump(runner.log))
	}
}

func TestInspectIgnoresCurrentGitmodulesWhenMissingFromBase(t *testing.T) {
	parent, _ := newRemoteRepo(t)
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitignore")
	runGit(t, parent, "commit", "-m", "base without submodules")
	runGit(t, parent, "push", "origin", "main")
	_, currentOrigin := newRemoteRepo(t)
	currentModules := "[submodule \"current\"]\n\tpath = modules/current\n\turl = " + currentOrigin + "\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(currentModules), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{inner: testRunner(t)}
	_, err := Inspect(context.Background(), runner, InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task",
		Worktree: filepath.Join(parent, ".worktrees", "task"), Submodules: []string{"modules/current"},
	})
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("Inspect() error = %v, want selected-base declaration error", err)
	}
	if runner.containsSequence("ls-remote", "--symref", "--", currentOrigin, "HEAD") {
		t.Fatalf("Inspect contacted remote absent from selected base:\n%s", commandDump(runner.log))
	}
}

func TestResolveRelativeRemoteURLKinds(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		relative string
		want     string
	}{
		{name: "https", base: "https://example.com/group/parent.git", want: "https://example.com/group/child.git"},
		{name: "ssh", base: "ssh://git@example.com/group/parent.git", want: "ssh://git@example.com/group/child.git"},
		{name: "scp", base: "git@example.com:group/parent.git", want: "git@example.com:group/child.git"},
		{name: "file URL", base: "file:///srv/group/parent.git", want: "file:///srv/group/child.git"},
		{name: "local path", base: "/srv/group/parent.git", want: "/srv/group/child.git"},
		{name: "URL semantics", base: "https://example.com/group/parent.git?old=1#old", relative: "../child%20repo.git?new=1#new", want: "https://example.com/group/child%20repo.git?new=1#new"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relative := test.relative
			if relative == "" {
				relative = "../child.git"
			}
			got, err := resolveRelativeRemoteURL("/checkout/super", test.base, relative)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("resolveRelativeRemoteURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInspectRelativeSubmoduleRequiresOrigin(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	runGit(t, "", "init", "--initial-branch=main", repo)
	configureIdentity(t, repo)
	modules := "[submodule \"child\"]\n\tpath = modules/child\n\turl = ../child.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte(modules), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitmodules", ".gitignore")
	runGit(t, repo, "commit", "-m", "add relative submodule")

	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: repo, Base: "main", Branch: "feature/task",
		Worktree: filepath.Join(repo, ".worktrees", "task"), Submodules: []string{"modules/child"},
	})
	if err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("Inspect() error = %v, want relative URL origin error", err)
	}
}

func TestInspectRejectsSubmoduleURLGitOptionInjection(t *testing.T) {
	parent, _ := newRemoteRepo(t)
	_, childOrigin := newRemoteRepo(t)
	headRepo := filepath.Join(parent, "HEAD")
	runGit(t, "", "clone", "--bare", childOrigin, headRepo)
	sentinel := filepath.Join(t.TempDir(), "upload-pack-ran")
	uploadPack := filepath.Join(t.TempDir(), "upload-pack")
	script := "#!/bin/sh\n: > \"" + sentinel + "\"\nexec git-upload-pack \"$@\"\n"
	if err := os.WriteFile(uploadPack, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	modules := "[submodule \"evil\"]\n\tpath = modules/evil\n\turl = --upload-pack=" + uploadPack + "\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(modules), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\nHEAD/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitmodules", ".gitignore")
	runGit(t, parent, "commit", "-m", "add hostile submodule URL")
	runGit(t, parent, "push", "origin", "main")

	_, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task",
		Worktree: filepath.Join(parent, ".worktrees", "task"), Submodules: []string{"modules/evil"},
	})
	if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
		t.Fatalf("Git option injection executed upload-pack sentinel: %v", statErr)
	}
	if err == nil || !strings.Contains(err.Error(), "URL") {
		t.Fatalf("Inspect() error = %v, want unsafe URL rejection", err)
	}
}

func TestInspectAllowsGitSupportedURLCharacters(t *testing.T) {
	parent, _ := newRemoteRepo(t)
	_, firstOrigin := newRemoteRepo(t)
	localOrigin := filepath.Join(filepath.Dir(firstOrigin), "child%#?.git")
	if err := os.Rename(firstOrigin, localOrigin); err != nil {
		t.Fatal(err)
	}
	_, secondOrigin := newRemoteRepo(t)
	fileOrigin := filepath.Join(filepath.Dir(secondOrigin), "child space.git")
	if err := os.Rename(secondOrigin, fileOrigin); err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: fileOrigin}).String()
	modules := "[submodule \"local\"]\n\tpath = modules/local\n\turl = \"" + localOrigin + "\"\n" +
		"[submodule \"file\"]\n\tpath = modules/file\n\turl = \"" + fileURL + "\"\n"
	if err := os.WriteFile(filepath.Join(parent, ".gitmodules"), []byte(modules), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, parent, "add", ".gitmodules", ".gitignore")
	runGit(t, parent, "commit", "-m", "add Git-supported URLs")
	runGit(t, parent, "push", "origin", "main")

	snapshot, err := Inspect(context.Background(), testRunner(t), InspectRequest{
		Repo: parent, Base: "origin/main", Branch: "feature/task", Worktree: filepath.Join(parent, ".worktrees", "task"),
		Submodules: []string{"modules/local", "modules/file"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Submodules) != 2 || snapshot.Submodules[0].URL != localOrigin || snapshot.Submodules[1].URL != fileURL {
		t.Fatalf("Git-supported URLs changed in plan: %+v", snapshot.Submodules)
	}
}

func TestValidateRemoteURLRejectsOnlyArgvBoundaryRisks(t *testing.T) {
	for _, value := range []string{"-repository", "--upload-pack=evil", "repo.git\nnext", "repo.git\x00next", "repo.git\tnext"} {
		if err := validateRemoteURL(value); err == nil {
			t.Errorf("validateRemoteURL(%q) succeeded, want rejection", value)
		}
	}
	for _, value := range []string{"https://example.com/repo.git?x=1", "ssh://example.com/repo.git#fragment", "file:///tmp/child%20repo.git", "/tmp/child%#?.git", `..\repo.git`} {
		if err := validateRemoteURL(value); err != nil {
			t.Errorf("validateRemoteURL(%q) = %v, want Git-supported value", value, err)
		}
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
	if err := CreateWorktree(context.Background(), testRunner(t), Operation{Repo: parent, Path: worktree, Branch: "feature/task", BaseSHA: base}); err != nil {
		t.Fatal(err)
	}

	firstSHA := runGit(t, first, "rev-parse", "HEAD")
	operations := []SubmoduleOperation{{Path: "modules/first", URL: firstOrigin, Branch: "feature/task", BaseRef: "main", BaseSHA: firstSHA}}
	if err := PrepareSubmodules(context.Background(), testRunner(t), worktree, operations); err != nil {
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

	t.Run("symlink target is never executed", func(t *testing.T) {
		worktree := t.TempDir()
		if err := os.MkdirAll(filepath.Join(worktree, "scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(t.TempDir(), "external-ran")
		external := filepath.Join(t.TempDir(), "external-setup.sh")
		script := "#!/bin/sh\n: > \"" + sentinel + "\"\n"
		if err := os.WriteFile(external, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(worktree, "scripts", "setup-worktree.sh")); err != nil {
			t.Fatal(err)
		}
		err := RunSetup(context.Background(), worktree, nil)
		if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
			t.Fatalf("RunSetup executed external symlink target: %v", statErr)
		}
		if err == nil || !strings.Contains(err.Error(), "regular") {
			t.Fatalf("RunSetup() error = %v, want symlink rejection", err)
		}
	})
}
