package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runGit 는 테스트 셋업용으로 dir 에서 git 을 실행하고 표준 출력을 돌려준다.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 실패: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// setupRepo 는 main 브랜치에 커밋 하나가 있는 임시 git 저장소를 만든다.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@t.t")
	runGit(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a")
	runGit(t, dir, "commit", "-qm", "init")
	return dir
}

// makeBrokenBranch 는 객체가 유실돼 메타데이터를 읽을 수 없는 브랜치를 만든다.
// 실제 손상 저장소(commit 객체 누락)를 재현한다.
func makeBrokenBranch(t *testing.T, dir, name string) {
	t.Helper()
	runGit(t, dir, "checkout", "-q", "-b", name)
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "x")
	runGit(t, dir, "commit", "-qm", "doomed")
	oid := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	runGit(t, dir, "checkout", "-q", "main")
	obj := filepath.Join(dir, ".git", "objects", oid[:2], oid[2:])
	if err := os.Remove(obj); err != nil {
		t.Fatalf("커밋 객체 삭제 실패(%s): %v", oid, err)
	}
}

// chdir 는 테스트 동안 작업 디렉터리를 dir 로 바꾸고 종료 시 복원한다.
// gitx.run 이 cwd 의 git 저장소를 대상으로 하기 때문에 필요하다.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func refNames(refs []BranchRef) []string {
	var names []string
	for _, r := range refs {
		names = append(names, r.Name)
	}
	return names
}

// TestLocalBranchesSkipsBrokenRefs 는 객체가 유실된 브랜치가 있어도
// LocalBranches 가 전체를 중단하지 않고 정상 브랜치를 돌려주며, 깨진 브랜치는
// broken 목록으로 분리하는지 검증한다. 깨진 브랜치 하나가 전체 분류를 막으면
// 안 된다는 불변조건을 고정한다.
func TestLocalBranchesSkipsBrokenRefs(t *testing.T) {
	dir := setupRepo(t)
	runGit(t, dir, "branch", "feature/good")
	makeBrokenBranch(t, dir, "feature/broken")
	chdir(t, dir)

	refs, broken, err := LocalBranches()
	if err != nil {
		t.Fatalf("깨진 브랜치 때문에 전체가 실패하면 안 된다: %v", err)
	}
	names := refNames(refs)
	if !contains(names, "main") || !contains(names, "feature/good") {
		t.Errorf("정상 브랜치(main, feature/good)가 결과에 있어야 한다, got=%v", names)
	}
	if contains(names, "feature/broken") {
		t.Errorf("깨진 브랜치는 정상 목록에서 빠져야 한다, got=%v", names)
	}
	if !contains(broken, "feature/broken") {
		t.Errorf("깨진 브랜치는 broken 목록으로 보고돼야 한다, got=%v", broken)
	}
}

// TestLocalBranchesHealthyRepo 는 깨진 브랜치가 없을 때 모든 브랜치를 돌려주고
// broken 이 비어 있는지 검증한다(빠른 경로 회귀 방지).
func TestLocalBranchesHealthyRepo(t *testing.T) {
	dir := setupRepo(t)
	runGit(t, dir, "branch", "feature/x")
	chdir(t, dir)

	refs, broken, err := LocalBranches()
	if err != nil {
		t.Fatalf("정상 저장소에서 실패하면 안 된다: %v", err)
	}
	if len(broken) != 0 {
		t.Errorf("정상 저장소의 broken 은 비어야 한다, got=%v", broken)
	}
	names := refNames(refs)
	if !contains(names, "main") || !contains(names, "feature/x") {
		t.Errorf("모든 브랜치가 있어야 한다, got=%v", names)
	}
}

// TestRunIncludesStderrInError 는 git 이 실패할 때 run 이 stderr 메시지를
// 에러에 포함하는지 검증한다(불투명한 "exit status 128" 방지).
func TestRunIncludesStderrInError(t *testing.T) {
	dir := setupRepo(t)
	chdir(t, dir)

	_, err := run("rev-parse", "--verify", "no-such-ref-xyz")
	if err == nil {
		t.Fatal("존재하지 않는 ref 조회는 에러여야 한다")
	}
	if !strings.Contains(err.Error(), "fatal") && !strings.Contains(err.Error(), "Needed a single revision") {
		t.Errorf("에러에 git stderr 가 포함돼야 한다, got=%q", err.Error())
	}
}

func TestParseBranchLines(t *testing.T) {
	// for-each-ref --format='%(refname:short)\x00%(upstream:track)\x00%(committerdate:unix)\x00%(subject)'
	input := "feature-a\x00[gone]\x001700000000\x00[ABC-1] gone branch\n" +
		"feature-b\x00\x001800000000\x00[ABC-2] no upstream\n" +
		"feature-c\x00[ahead 1]\x001900000000\x00[ABC-3] ahead branch\n"
	got := parseBranchLines(input)
	want := []BranchRef{
		{Name: "feature-a", UpstreamGone: true, HasUpstream: true, CommitUnix: 1700000000, Subject: "[ABC-1] gone branch"},
		{Name: "feature-b", UpstreamGone: false, HasUpstream: false, CommitUnix: 1800000000, Subject: "[ABC-2] no upstream"},
		{Name: "feature-c", UpstreamGone: false, HasUpstream: true, CommitUnix: 1900000000, Subject: "[ABC-3] ahead branch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranchLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseCommitLines(t *testing.T) {
	input := "9a640b52f\x001780376000\x00[ABC-1375] feat: 새 worktree 셋업\n" +
		"4d4f1d52f\x001780635662\x00[ABC-1399] fix(ci): landing-lighthouse\n"
	got := parseCommitLines(input)
	want := []CommitRef{
		{ShortHash: "9a640b52f", CommitUnix: 1780376000, Subject: "[ABC-1375] feat: 새 worktree 셋업"},
		{ShortHash: "4d4f1d52f", CommitUnix: 1780635662, Subject: "[ABC-1399] fix(ci): landing-lighthouse"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseCommitLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseWorktreeBranches(t *testing.T) {
	input := "worktree /a\nbranch refs/heads/main\n\nworktree /b\nbranch refs/heads/feature-x\n\nworktree /c\ndetached\n"
	got := parseWorktreeBranches(input)
	want := map[string]string{"main": "/a", "feature-x": "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktreeBranches mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

// TestRemoveWorktreeRecoversBrokenLink 는 worktree 의 .git 링크 파일이 유실된
// "깨진 worktree" 를 RemoveWorktree 가 회복하는지 검증한다. 이 상태에서는 git 의
// worktree remove 가 (--force 포함) 유효성 검사로 거부하고, prune 은 디렉터리가
// 아직 존재해 대상으로 삼지 않는 사각지대가 된다. RemoveWorktree 가 디렉터리를
// 직접 지우고 prune 으로 admin 메타까지 정리해 빠져나오는 불변조건을 고정한다.
func TestRemoveWorktreeRecoversBrokenLink(t *testing.T) {
	dir := setupRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", "-q", "-b", "feature/x", wt)
	// .git 링크 파일을 지워 깨진 worktree 를 재현한다.
	if err := os.Remove(filepath.Join(wt, ".git")); err != nil {
		t.Fatalf(".git 링크 삭제 실패: %v", err)
	}
	chdir(t, dir)

	if err := RemoveWorktree(wt); err != nil {
		t.Fatalf("깨진 worktree 를 회복해야 한다: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree 디렉터리가 제거돼야 한다: stat err=%v", err)
	}
	if out := runGit(t, dir, "worktree", "list", "--porcelain"); strings.Contains(out, wt) {
		t.Errorf("worktree 목록에서 사라져야 한다(admin 메타 정리):\n%s", out)
	}
}

// TestRemoveWorktreeKeepsHealthyOnFailure 는 .git 링크가 정상인 worktree 가
// 미커밋 변경 때문에 remove 거부될 때, RemoveWorktree 가 에러를 그대로 돌려주고
// 디렉터리를 보존하는지 검증한다. 회복 폴백이 건강한 worktree 의 사용자 작업을
// 지우지 않는다는 안전 불변조건을 고정한다.
func TestRemoveWorktreeKeepsHealthyOnFailure(t *testing.T) {
	dir := setupRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, dir, "worktree", "add", "-q", "-b", "feature/y", wt)
	// 추적 파일을 수정해 dirty 상태로 만들면 remove 가 거부한다.
	if err := os.WriteFile(filepath.Join(wt, "a"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	if err := RemoveWorktree(wt); err == nil {
		t.Fatal("dirty 한 정상 worktree 의 remove 는 에러여야 한다")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("정상 worktree 디렉터리는 보존돼야 한다: %v", err)
	}
}
