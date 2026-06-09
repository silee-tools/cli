package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestShouldPinMain(t *testing.T) {
	cases := []struct {
		name          string
		cwd, mainPath string
		want          bool
	}{
		{"subdir pins", "/repo/sub", "/repo", true},
		{"at main root no pin", "/repo", "/repo", false},
		{"empty main no pin", "/repo/sub", "", false},
	}
	for _, tc := range cases {
		if got := shouldPinMain(tc.cwd, tc.mainPath); got != tc.want {
			t.Errorf("%s: shouldPinMain(%q, %q) = %v, want %v",
				tc.name, tc.cwd, tc.mainPath, got, tc.want)
		}
	}
}

// newGitRepo 는 커밋 한 개를 가진 임시 git 저장소를 만들고 경로를 돌려준다.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestResolvePinnedMain(t *testing.T) {
	repo := newGitRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// cwd 가 저장소 하위 디렉토리이면 저장소 루트를 고정 대상으로 돌려준다.
	if got := resolvePinnedMain(sub); canonicalPath(got) != canonicalPath(repo) {
		t.Errorf("resolvePinnedMain(sub) = %q, want repo root %q", got, repo)
	}

	// cwd 가 곧 main 루트이면 빈 문자열.
	if got := resolvePinnedMain(repo); got != "" {
		t.Errorf("resolvePinnedMain(root) = %q, want empty", got)
	}

	// git 저장소 밖이면 빈 문자열.
	nonRepo := t.TempDir()
	if got := resolvePinnedMain(nonRepo); got != "" {
		t.Errorf("resolvePinnedMain(nonRepo) = %q, want empty", got)
	}
}
