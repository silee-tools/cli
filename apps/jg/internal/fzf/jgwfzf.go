package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/silee-tools/jg/internal/worktree"
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

// worktreeLabel 은 worktree 한 개를 picker 에 보여줄 한 줄 라벨로 만든다.
// 이름은 경로 basename 이고, 원본(main) 은 "▸ " 마커를, 나머지는 같은 폭의
// 공백을 앞에 둔다. 브랜치 basename 이 이름과 같으면 중복이므로 브랜치를
// 생략하고, 다르면 이름 뒤에 공백 두 칸으로 브랜치를 잇는다. 브랜치가 없는
// detached worktree 는 "(detached)" 를 덧붙인다.
// 수정 시 검토 관점: 이 라벨은 buildWorktreeInput 이 "<인덱스>\t<라벨>" 로
// 엮어 fzf 에 넘기고 fzf 는 라벨만 보여주므로, 라벨 안에 탭 문자를 넣지 않는다.
func worktreeLabel(w worktree.Worktree) string {
	name := filepath.Base(w.Path)
	marker := "  "
	if w.IsMain {
		marker = "▸ "
	}
	if w.Branch == "" {
		return marker + name + "  (detached)"
	}
	if path.Base(w.Branch) == name {
		return marker + name
	}
	return marker + name + "  " + w.Branch
}

// worktreePreviewCmd 는 focused 항목의 브랜치/마지막 커밋 제목/마지막 커밋 시각을
// 표시하는 preview 명령을 만든다.
func worktreePreviewCmd(home string) string {
	// fzf 가 {} 를 작은따옴표로 감싸 치환하므로 여기서 다시 따옴표로 감싸지
	// 않는다. leading ~ 만 home 경로로 치환하되 dash 같은 POSIX sh 에서도
	// 동작하도록 case 와 ${p#~} 만 쓴다. ${p/.../...} 는 bash·zsh 전용이라
	// dash 에서 "Bad substitution" 으로 깨진다.
	resolve := fmt.Sprintf(`p={}; case "$p" in "~"*) p="%s${p#\~}";; esac`, home)
	return resolve + `; echo "branch: $(git -C "$p" symbolic-ref --short HEAD 2>/dev/null || echo "(detached)")"; echo; git -C "$p" log -1 --format='subject: %s%ndate:    %cd' --date=format:'%Y-%m-%d %H:%M' 2>/dev/null`
}
