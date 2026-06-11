package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
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
	// git ref 는 OS 와 무관하게 '/' 구분자를 쓰므로 filepath 가 아니라 path.Base 로 짧은 이름을 뗀다.
	if path.Base(w.Branch) == name {
		return marker + name
	}
	return marker + name + "  " + w.Branch
}

// WorktreeListPickerInput 은 worktree 선택 단계 전용 picker 호출 파라미터다.
// 후보는 경로 문자열이 아니라 worktree 구조체로 받아, 라벨을 이름 중심으로
// 그리고 선택 결과는 인덱스로 역조회해 경로를 돌려준다.
type WorktreeListPickerInput struct {
	Candidates []worktree.Worktree // 선택 가능한 worktree 후보 (현재 위치 제외)
	Current    *worktree.Worktree  // 현재 위치한 worktree. nil 이 아니면 헤더 줄로 고정 표시
	StepHeader string              // "[1/1 worktree 선택]" / "[2/2 worktree 선택]"
	OriginLine string              // "원본: <path> (<branch>)"
}

// buildWorktreeInput 은 fzf 에 넘길 입력 문자열과 헤더 줄 수를 만든다. 각 줄은
// "<인덱스>\t<라벨>" 이며, 인덱스는 Candidates 슬라이스의 자리값이다. Current 가
// 있으면 맨 앞에 인덱스 -1 의 헤더 줄을 두고 headerLines 를 1 로 돌려준다(이 줄은
// fzf 의 --header-lines 로 고정돼 선택되지 않으므로 인덱스 값은 쓰이지 않는다).
// StepHeader·OriginLine 은 RunWorktreeListPicker 가 fzf 헤더로 쓰며 이 함수는 읽지 않는다.
func buildWorktreeInput(in WorktreeListPickerInput) (input string, headerLines int) {
	var b strings.Builder
	if in.Current != nil {
		fmt.Fprintf(&b, "-1\t%s\n", worktreeLabel(*in.Current))
		headerLines = 1
	}
	for i, w := range in.Candidates {
		fmt.Fprintf(&b, "%d\t%s\n", i, worktreeLabel(w))
	}
	return b.String(), headerLines
}

// selectedWorktreeIndex 는 fzf 가 돌려준 선택 줄에서 맨 앞 인덱스 필드를 떼어
// 정수로 바꾼다. 줄은 "<인덱스>\t<라벨>" 형식이라 첫 탭 앞을 인덱스로 본다.
// 비어 있거나 인덱스가 정수가 아니면 ok=false 를 돌려준다.
func selectedWorktreeIndex(selected string) (int, bool) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return 0, false
	}
	field := selected
	if i := strings.Index(selected, "\t"); i >= 0 {
		field = selected[:i]
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0, false
	}
	return n, true
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
