package fzf

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// fzfSubstitute 는 fzf 의 {} placeholder 치환을 흉내낸다. fzf 는 preview 명령을
// 실행하기 전에 {} 를 현재 줄의 작은따옴표로 감싼 문자열로 바꾼다.
func fzfSubstitute(cmd, line string) string {
	return strings.ReplaceAll(cmd, "{}", "'"+line+"'")
}

// previewShell 은 preview 명령을 실행할 셸을 고른다. dash 가 있으면 dash 로
// 실행해 bash·zsh 전용 문법(bashism)을 개발 환경에서도 잡아낸다. fzf 가 깔린
// 일부 환경의 /bin/sh 는 bash 라 bashism 을 눈감아 주기 때문이다. dash 가
// 없으면 sh 로 떨어진다.
func previewShell() string {
	if p, err := exec.LookPath("dash"); err == nil {
		return p
	}
	return "sh"
}

// newPreviewRepo 는 주어진 브랜치 위에 커밋 한 개를 가진 임시 git repo 를
// 만들고 그 절대 경로를 반환한다.
func newPreviewRepo(t *testing.T, branch, subject string) string {
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
	run("checkout", "-b", branch)
	run("commit", "--allow-empty", "-m", subject)
	return dir
}

// TestWorktreePreviewCmdResolvesFocusedPath 는 jgw picker 의 preview 명령이
// fzf 가 넘긴 focused path 를 그대로 git 에 전달해 브랜치/커밋 제목/날짜를
// 출력하는지 검증한다. preview 명령이 {} 를 따옴표로 한 번 더 감싸면 경로에
// 따옴표 문자가 섞여 git 이 경로를 못 찾고 (detached) 로 잘못 표시된다.
func TestWorktreePreviewCmdResolvesFocusedPath(t *testing.T) {
	repo := newPreviewRepo(t, "probe-branch", "preview probe commit")
	cmd := fzfSubstitute(worktreePreviewCmd("/home/unused"), repo)
	out, _ := exec.Command(previewShell(), "-c", cmd).CombinedOutput()
	got := string(out)
	for _, want := range []string{
		"branch: probe-branch",
		"subject: preview probe commit",
		"date:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("preview output missing %q\n--- got ---\n%s", want, got)
		}
	}
	if strings.Contains(got, "(detached)") {
		t.Errorf("preview reported (detached) for a repo on a branch\n--- got ---\n%s", got)
	}
}

// TestPreviewCmdResolvesFocusedPath 는 jg repo picker 의 preview 명령이 동일한
// 따옴표 처리 문제 없이 focused repo 의 브랜치와 커밋을 출력하는지 검증한다.
func TestPreviewCmdResolvesFocusedPath(t *testing.T) {
	repo := newPreviewRepo(t, "probe-branch", "preview probe commit")
	cmd := strings.ReplaceAll(previewCmd("/home/unused"), "{2}", "'"+repo+"'")
	out, _ := exec.Command(previewShell(), "-c", cmd).CombinedOutput()
	got := string(out)
	if !strings.Contains(got, "branch: probe-branch") {
		t.Errorf("preview output missing branch line\n--- got ---\n%s", got)
	}
	if !strings.Contains(got, "preview probe commit") {
		t.Errorf("preview output missing commit subject\n--- got ---\n%s", got)
	}
}
