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
