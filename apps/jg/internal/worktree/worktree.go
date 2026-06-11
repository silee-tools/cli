package worktree

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

type Worktree struct {
	Path   string
	Branch string // git ref 의 refs/heads/ 접두사를 제외한 짧은 브랜치명. detached 면 빈 문자열.
	IsMain bool
}

func parsePorcelain(out []byte) ([]Worktree, error) {
	var result []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" {
			result = append(result, cur)
		}
		cur = Worktree{}
	}
	first := true
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
			cur.IsMain = first
			first = false
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			// branch 는 빈 문자열 유지
		}
	}
	flush()
	return result, scanner.Err()
}

// List 는 주어진 repo 경로의 worktree 목록을 반환한다.
// repoPath 는 worktree 안의 임의 디렉토리여도 git 이 알아서 main 을 찾는다.
func List(repoPath string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out)
}
