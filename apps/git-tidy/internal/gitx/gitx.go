// Package gitx 는 git CLI 를 호출해 git-tidy 가 쓰는 정보를 모은다.
package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// branchFormat 은 LocalBranches 가 브랜치 메타데이터를 한 줄로 받기 위한 형식이다.
// 단일 호출(빠른 경로)과 브랜치별 호출(복구 경로)이 같은 파서를 공유하도록 상수로 둔다.
const branchFormat = "%(refname:short)%00%(upstream:track)%00%(committerdate:unix)%00%(subject)"

// BranchRef 는 로컬 브랜치 하나의 git 메타데이터다.
type BranchRef struct {
	Name         string
	HasUpstream  bool // upstream 추적 브랜치가 설정돼 있는가
	UpstreamGone bool // upstream 이 [gone] 인가
	CommitUnix   int64
	Subject      string
}

// CommitRef 는 base 브랜치의 커밋 하나에 대한 분류용 메타데이터다.
type CommitRef struct {
	ShortHash  string
	Subject    string
	CommitUnix int64
}

// run 은 git 을 실행하고 표준 출력을 돌려준다. 실패 시 git 의 stderr 를 에러에
// 포함해, 호출자가 불투명한 "exit status N" 대신 실제 원인(예: missing object)을
// 볼 수 있게 한다.
func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
		}
	}
	return stdout.String(), err
}

// IsRepo 는 현재 디렉터리가 git 저장소인지 본다.
func IsRepo() bool {
	_, err := run("rev-parse", "--git-dir")
	return err == nil
}

// FetchPrune 는 git fetch --prune 를 실행한다. 실패는 무시한다(오프라인 등).
func FetchPrune() {
	_, _ = run("fetch", "--prune")
}

// CurrentBranch 는 체크아웃된 브랜치 이름을 돌려준다(detached 면 빈 문자열).
func CurrentBranch() string {
	out, _ := run("branch", "--show-current")
	return strings.TrimSpace(out)
}

// LocalBranches 는 모든 로컬 브랜치의 메타데이터를 돌려준다.
// 정상 브랜치는 refs 로, 객체가 유실돼 메타데이터를 읽을 수 없는 브랜치 이름은
// broken 으로 분리한다. 깨진 브랜치 하나가 전체 조회를 막지 않게 하기 위함이다.
//
// 단일 for-each-ref 는 브랜치 tip 의 커밋 객체가 하나라도 없으면 전체가 exit 128
// 로 죽으므로(이 형식이 committerdate·subject 를 읽어야 하기 때문), 빠른 경로가
// 실패하면 refname 만 받아(객체 불필요) 브랜치별로 다시 조회해 깨진 것만 건너뛴다.
func LocalBranches() ([]BranchRef, []string, error) {
	if out, err := run("for-each-ref", "--format="+branchFormat, "refs/heads"); err == nil {
		return parseBranchLines(out), nil, nil
	}
	return localBranchesResilient()
}

// localBranchesResilient 는 손상된 저장소용 복구 경로다. refname 만 먼저 받아
// (객체가 없어도 안전) 브랜치별로 메타데이터를 조회하고, 조회가 실패하는 브랜치는
// broken 으로 모은다.
func localBranchesResilient() ([]BranchRef, []string, error) {
	out, err := run("for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, nil, err
	}
	var refs []BranchRef
	var broken []string
	for _, name := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if name == "" {
			continue
		}
		line, err := run("for-each-ref", "--format="+branchFormat, "refs/heads/"+name)
		parsed := parseBranchLines(line)
		if err != nil || len(parsed) == 0 {
			broken = append(broken, name)
			continue
		}
		refs = append(refs, parsed...)
	}
	return refs, broken, nil
}

func parseBranchLines(out string) []BranchRef {
	var refs []BranchRef
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 4 {
			continue
		}
		unix, _ := strconv.ParseInt(f[2], 10, 64)
		track := f[1]
		refs = append(refs, BranchRef{
			Name:         f[0],
			HasUpstream:  track != "",
			UpstreamGone: track == "[gone]",
			CommitUnix:   unix,
			Subject:      f[3],
		})
	}
	return refs
}

// BaseCommits 는 base 브랜치의 커밋 메타데이터를 최신순으로 돌려준다.
func BaseCommits(base string) ([]CommitRef, error) {
	out, err := run("log", "--format=%h%x00%ct%x00%s", base)
	if err != nil {
		return nil, err
	}
	return parseCommitLines(out), nil
}

func parseCommitLines(out string) []CommitRef {
	var refs []CommitRef
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 3 {
			continue
		}
		unix, _ := strconv.ParseInt(f[1], 10, 64)
		refs = append(refs, CommitRef{ShortHash: f[0], CommitUnix: unix, Subject: f[2]})
	}
	return refs
}

// WorktreeBranches 는 worktree 에 체크아웃된 브랜치 → worktree 경로 맵을 돌려준다.
func WorktreeBranches() (map[string]string, error) {
	out, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeBranches(out), nil
}

func parseWorktreeBranches(out string) map[string]string {
	result := map[string]string{}
	var path string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			result[strings.TrimPrefix(line, "branch refs/heads/")] = path
		}
	}
	return result
}

// BaseBranch 는 main/master/trunk 등 기본 브랜치를 자동 감지한다.
func BaseBranch() string {
	for _, name := range []string{"main", "master", "trunk"} {
		if _, err := run("show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	return "main"
}

// MergedBranches 는 base 에 머지된 로컬 브랜치 이름 집합을 돌려준다.
func MergedBranches(base string) (map[string]bool, error) {
	out, err := run("branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

// MergeBaseUnix 는 base 와 branch 의 분기점 커밋 시각(unix)을 돌려준다.
func MergeBaseUnix(base, branch string) (int64, bool) {
	mb, err := run("merge-base", base, branch)
	if err != nil {
		return 0, false
	}
	out, err := run("show", "-s", "--format=%ct", strings.TrimSpace(mb))
	if err != nil {
		return 0, false
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return unix, err == nil
}

// DeleteBranch 는 git branch -D 로 브랜치를 삭제한다.
func DeleteBranch(name string) error {
	_, err := run("branch", "-D", name)
	return err
}

// RemoveWorktree 는 worktree 를 제거한다. 미커밋 변경이 있으면 git 이 거부한다.
// worktree 의 .git 링크가 유실된 깨진 worktree 는 git worktree remove 가 (--force
// 포함) 유효성 검사로 거부하고, prune 은 디렉터리가 남아 있어 대상으로 삼지 않는
// 사각지대가 된다. 그 경우에 한해 디렉터리를 직접 지우고 prune 으로 admin 메타를
// 정리해 회복한다.
//
// 불변조건: os.RemoveAll 폴백은 .git 이 없을 때만 탄다. .git 이 정상인데 remove 가
// 거부됐다면(미커밋 변경 등) 디렉터리를 지우지 않고 에러를 그대로 돌려줘 사용자
// 작업 손실을 막는다 — 이 가드를 좁히면 건강한 worktree 를 파괴할 수 있다.
func RemoveWorktree(path string) error {
	_, removeErr := run("worktree", "remove", path)
	if removeErr == nil {
		return nil
	}
	if _, statErr := os.Stat(filepath.Join(path, ".git")); !os.IsNotExist(statErr) {
		return removeErr
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	_, err := run("worktree", "prune")
	return err
}
