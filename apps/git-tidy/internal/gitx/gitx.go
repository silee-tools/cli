// Package gitx 는 git CLI 를 호출해 git-tidy 가 쓰는 정보를 모은다.
package gitx

import (
	"os/exec"
	"strconv"
	"strings"
)

// BranchRef 는 로컬 브랜치 하나의 git 메타데이터다.
type BranchRef struct {
	Name         string
	HasUpstream  bool // upstream 추적 브랜치가 설정돼 있는가
	UpstreamGone bool // upstream 이 [gone] 인가
	CommitUnix   int64
}

// run 은 git 을 실행하고 표준 출력을 돌려준다.
func run(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return string(out), err
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
func LocalBranches() ([]BranchRef, error) {
	out, err := run("for-each-ref",
		"--format=%(refname:short)%00%(upstream:track)%00%(committerdate:unix)",
		"refs/heads")
	if err != nil {
		return nil, err
	}
	return parseBranchLines(out), nil
}

func parseBranchLines(out string) []BranchRef {
	var refs []BranchRef
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 3 {
			continue
		}
		unix, _ := strconv.ParseInt(f[2], 10, 64)
		track := f[1]
		refs = append(refs, BranchRef{
			Name:         f[0],
			HasUpstream:  track != "",
			UpstreamGone: track == "[gone]",
			CommitUnix:   unix,
		})
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
func RemoveWorktree(path string) error {
	_, err := run("worktree", "remove", path)
	return err
}
