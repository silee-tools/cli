// Package gitx 는 git CLI 를 호출해 git-update-default 가 쓰는 정보를 모으고
// 저장소를 조작한다.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// run 은 git 을 실행하고 표준 출력을 돌려준다. 실패 시 git 의 stderr 를 에러에
// 포함해, 호출자가 불투명한 "exit status N" 대신 실제 원인을 볼 수 있게 한다.
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

// IsRepo 는 현재 디렉터리가 git 작업 트리 안인지 본다. 하위 경로에서 실행해도
// git 이 상위로 거슬러 저장소를 찾으므로, 별도의 root 탐색 로직이 필요 없다.
// sibling git-tidy 의 --git-dir 과 달리 --is-inside-work-tree 를 쓰는 것은
// 의도적이다 — 이 도구는 작업 트리 안에서만 동작해야 하고(bare 저장소나 .git
// 디렉터리 내부는 대상이 아님), 그 경우 "true" 출력 가드가 더 좁게 막아 준다.
func IsRepo() bool {
	out, err := run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// HasOriginRemote 는 origin 원격이 설정돼 있는지 본다.
func HasOriginRemote() bool {
	_, err := run("remote", "get-url", "origin")
	return err == nil
}

// FetchPrune 는 origin 의 최신 상태를 받고 사라진 원격 브랜치를 정리한다.
// 실패(오프라인 등)는 에러로 돌려, 호출자가 경고할지 정한다.
func FetchPrune() error {
	_, err := run("fetch", "origin", "--prune")
	return err
}

// RemoteBranchExists 는 origin/<name> 원격 추적 ref 가 있는지 본다.
func RemoteBranchExists(name string) bool {
	_, err := run("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+name)
	return err == nil
}

// SymbolicRefDefault 는 origin/HEAD 가 가리키는 default branch 이름을 돌려준다.
// refs/remotes/origin/HEAD 가 없으면 set-head 로 한 번 갱신을 시도한 뒤 다시 읽는다.
func SymbolicRefDefault() (string, bool) {
	read := func() (string, bool) {
		out, err := run("symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return "", false
		}
		name := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
		return name, name != ""
	}
	if name, ok := read(); ok {
		return name, true
	}
	_, _ = run("remote", "set-head", "origin", "-a")
	return read()
}

// CurrentBranch 는 체크아웃된 브랜치 이름을 돌려준다(detached 면 빈 문자열).
func CurrentBranch() string {
	out, _ := run("branch", "--show-current")
	return strings.TrimSpace(out)
}

// DirtyFiles 는 커밋되지 않은 변경 파일을 git status --porcelain 형식 줄로
// 돌려준다(추적되지 않는 파일 포함). 비어 있으면 작업 트리가 clean 이다.
func DirtyFiles() ([]string, error) {
	out, err := run("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// StashPush 는 추적되지 않는 파일까지 포함해 작업 트리 변경을 보관한다.
// 불변조건: 호출자는 DirtyFiles 가 비어 있지 않을 때만 호출한다. clean 저장소에서
// 부르면 git 이 아무것도 보관하지 않고 성공하므로, 뒤이은 stash pop 이 의도치 않은
// 다른 stash 를 꺼낼 수 있다.
func StashPush() error {
	msg := "git-update-default " + time.Now().Format(time.RFC3339)
	_, err := run("stash", "push", "-u", "-m", msg)
	return err
}

// ResetHard 는 추적되는 파일의 커밋되지 않은 변경을 버린다. 추적되지 않는
// 새 파일은 건드리지 않는다.
func ResetHard() error {
	_, err := run("reset", "--hard", "HEAD")
	return err
}

// LocalBranchExists 는 로컬에 <name> 브랜치가 있는지 본다.
func LocalBranchExists(name string) bool {
	_, err := run("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Switch 는 로컬 브랜치로 전환한다.
func Switch(name string) error {
	_, err := run("switch", name)
	return err
}

// SwitchCreateTracking 은 origin/<name> 을 추적하는 로컬 브랜치를 만들어 전환한다.
func SwitchCreateTracking(name string) error {
	_, err := run("switch", "-c", name, "--track", "origin/"+name)
	return err
}

// MergeFFOnly 는 현재 브랜치를 origin/<name> 까지 fast-forward 한다.
// 갈라져서 fast-forward 가 불가능하면 에러를 돌려준다(강제하지 않는다).
func MergeFFOnly(name string) error {
	_, err := run("merge", "--ff-only", "origin/"+name)
	return err
}
