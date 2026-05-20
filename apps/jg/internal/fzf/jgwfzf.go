package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// WorktreePickerInput 은 jgw 의 fzf picker 호출 파라미터다.
type WorktreePickerInput struct {
	Candidates  []string // 후보 path 들 (현재 위치 제외)
	CurrentPath string   // 현재 위치한 worktree path. 비어 있으면 header-lines 미사용
	StepHeader  string   // "[1/2 repo 선택]" / "[2/2 worktree 선택]"
	OriginLine  string   // "원본: <path> (<branch>)" — 흐름 b 의 2단계에서만 채움
}

// RunWorktreePicker 는 fzf 를 띄우고 선택된 path 를 반환한다.
// 취소 시 빈 문자열을 반환한다.
func RunWorktreePicker(in WorktreePickerInput) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("fzf not found. Install it: brew install fzf")
	}
	home, _ := os.UserHomeDir()

	headerParts := []string{}
	if in.StepHeader != "" {
		headerParts = append(headerParts, in.StepHeader)
	}
	if in.OriginLine != "" {
		headerParts = append(headerParts, in.OriginLine)
	}
	header := strings.Join(headerParts, "\n")

	args := []string{
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--select-1",
		"--keep-right",
		"--wrap",
		"--preview", worktreePreviewCmd(home),
	}
	if header != "" {
		args = append(args, "--header", header)
	}

	var input strings.Builder
	if in.CurrentPath != "" {
		fmt.Fprintln(&input, shortenPath(in.CurrentPath, home))
		args = append(args, "--header-lines=1")
	}
	for _, p := range in.Candidates {
		fmt.Fprintln(&input, shortenPath(p, home))
	}

	cmd := exec.Command(fzfPath, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", err
	}
	return expandPath(strings.TrimSpace(string(out)), home), nil
}

// worktreePreviewCmd 는 focused 항목의 브랜치/마지막 커밋 제목/마지막 커밋 시각을
// 표시하는 preview 명령을 만든다.
func worktreePreviewCmd(home string) string {
	resolve := fmt.Sprintf(`p="{}"; p="${p/#\\~/%s}"`, home)
	return resolve + `; echo "branch: $(git -C "$p" symbolic-ref --short HEAD 2>/dev/null || echo "(detached)")"; echo; git -C "$p" log -1 --format='subject: %s%ndate:    %cd' --date=format:'%Y-%m-%d %H:%M' 2>/dev/null`
}
