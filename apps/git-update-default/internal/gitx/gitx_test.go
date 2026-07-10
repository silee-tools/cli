package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInit 은 임시 디렉터리에 커밋 하나가 있는 git 저장소를 만들고, 그 안으로
// 작업 디렉터리를 옮긴다. 테스트가 끝나면 원래 디렉터리로 돌아간다.
func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	steps := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func TestIsRepoAndDirty(t *testing.T) {
	gitInit(t)

	if !IsRepo() {
		t.Fatal("IsRepo = false, want true in a git repo")
	}

	files, err := DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("clean repo DirtyFiles = %v, want empty", files)
	}

	if err := os.WriteFile("f.txt", []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err = DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("dirty repo DirtyFiles = %v, want 1 entry", files)
	}
}

func TestIsRepoFalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if IsRepo() {
		t.Fatal("IsRepo = true outside a repo, want false")
	}
}

func TestUpstreamAndMergeFFOnlyRef(t *testing.T) {
	// origin 역할을 할 bare 저장소를 만들고, gitInit 저장소를 그 origin 에 연결한다.
	upstreamDir := t.TempDir()
	for _, args := range [][]string{{"init", "--bare", "-b", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = upstreamDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	gitInit(t) // cwd = 커밋 하나 있는 작업 저장소
	mustGit(t, "remote", "add", "origin", upstreamDir)
	mustGit(t, "push", "-u", "origin", "main")

	// upstream 이 설정됐으니 Upstream 은 origin/main 을 돌려준다.
	up, err := Upstream()
	if err != nil {
		t.Fatalf("Upstream err = %v, want nil", err)
	}
	if up != "origin/main" {
		t.Fatalf("Upstream = %q, want origin/main", up)
	}

	// origin 을 한 커밋 앞서게 만들고(다른 클론에서 push), fetch 후 ff 되는지 본다.
	other := t.TempDir()
	mustGitDir(t, other, "clone", upstreamDir, ".")
	// GitHub Actions 러너처럼 전역 Git 사용자 정보가 없는 환경에서도
	// 테스트용 원격 커밋을 독립적으로 만들 수 있게 한다.
	mustGitDir(t, other, "config", "user.email", "test@example.com")
	mustGitDir(t, other, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(other, "g.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitDir(t, other, "add", ".")
	mustGitDir(t, other, "commit", "-m", "second")
	mustGitDir(t, other, "push", "origin", "main")

	if err := FetchPrune(); err != nil {
		t.Fatalf("FetchPrune err = %v", err)
	}
	if err := MergeFFOnlyRef(up); err != nil {
		t.Fatalf("MergeFFOnlyRef(%q) err = %v, want nil", up, err)
	}
	// ff 후 g.txt 가 존재해야 한다.
	if _, err := os.Stat("g.txt"); err != nil {
		t.Fatalf("ff 후 g.txt 없음: %v", err)
	}

	// upstream 이 없는 브랜치로 옮기면 Upstream 은 에러.
	mustGit(t, "switch", "-c", "no-upstream")
	if _, err := Upstream(); err == nil {
		t.Fatal("Upstream on branch without upstream = nil err, want error")
	}
}

// mustGit 은 현재 cwd 에서 git 을 실행하고 실패 시 종료한다.
func mustGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// mustGitDir 은 지정한 디렉터리에서 git 을 실행하고 실패 시 종료한다.
func mustGitDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git -C %s %v: %v: %s", dir, args, err, out)
	}
}
