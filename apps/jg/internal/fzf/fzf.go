package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/silee-tools/jg/internal/entry"
)

// Run launches fzf with the given entries and optional query. pinnedMain 이
// 비어 있지 않으면 그 경로를 라벨과 함께 피커 최상단에 고정한다.
// Returns the selected path or empty string if cancelled.
func Run(entries []entry.Entry, query, pinnedMain string) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("fzf not found. Install it: brew install fzf")
	}

	home, _ := os.UserHomeDir()
	lines := buildPickerLines(entries, pinnedMain, home)

	args := []string{
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--select-1",
		"--keep-right",
		"--wrap",
		"--delimiter=\t",
		"--with-nth=1",
		"--header=Git Repos",
		"--preview", previewCmd(home),
	}
	if query != "" {
		args = append(args, "--query", shortenPath(query, home))
	}

	cmd := exec.Command(fzfPath, args...)
	cmd.Stderr = os.Stderr

	var input strings.Builder
	for _, ln := range lines {
		fmt.Fprintf(&input, "%s\t%s\n", ln.display, ln.pathField)
	}
	cmd.Stdin = strings.NewReader(input.String())

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf exit 1 = no match, exit 130 = cancelled
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", err
	}

	return parseSelectedPath(string(out), home), nil
}

// pickerLine 은 fzf 한 줄의 표시 영역과 경로 영역을 나눠 담는다. 호출부가
// 둘을 탭으로 이어 fzf 에 넘기면, fzf 는 표시 영역만 보여주고 선택 줄에서
// 경로 영역만 떼어낸다.
type pickerLine struct {
	display   string
	pathField string
}

// buildPickerLines 는 fzf 에 넘길 줄 목록을 만든다. pinnedMain 이 비어 있지
// 않으면 그 경로를 라벨과 함께 맨 앞에 고정하고, 본문 목록에서 같은 경로를
// 제거해 중복을 막는다.
// 수정 시 검토 관점:
//   - 라벨 문구("↑ main  ")를 바꾸면 표시만 바뀌고 반환 경로는 pathField 에서
//     떼므로 parseSelectedPath 와 짝이 깨지지 않는다. 라벨 끝 공백 2개는 화살표
//     뒤 경로를 일반 항목과 시각적으로 띄우는 패딩이므로 개수를 임의로 줄이지 않는다.
//   - display 와 pathField 를 호출부(Run)가 탭으로 이어 fzf 에 넘기고, fzf 는
//     --delimiter=\t --with-nth=1 로 display 만 보여주며 preview 는 경로 필드({2})를
//     본다. 이 탭 구분은 호출부 입력 포맷·fzf 옵션·parseSelectedPath 와 한 묶음이다.
//   - 중복 제거는 e.Path == pinnedMain 의 raw 비교로 충분하다. 두 값 모두 git 이
//     돌려준 경로(entry 는 rev-parse --show-toplevel, pinnedMain 은 worktree list)라
//     같은 정규화 도메인이며, 심볼릭 링크 cwd 로 접근해도 git 이 동일 문자열을
//     돌려준다. (splitCurrent 가 canonicalPath 로 정규화하는 것은 git 경로를 셸의
//     미해석 os.Getwd() 와 비교하기 때문이라 사정이 다르다.)
func buildPickerLines(entries []entry.Entry, pinnedMain, home string) []pickerLine {
	var lines []pickerLine
	if pinnedMain != "" {
		short := shortenPath(pinnedMain, home)
		lines = append(lines, pickerLine{
			display:   "↑ main  " + short,
			pathField: short,
		})
	}
	for _, e := range entries {
		if pinnedMain != "" && e.Path == pinnedMain {
			continue
		}
		short := shortenPath(e.Path, home)
		lines = append(lines, pickerLine{display: short, pathField: short})
	}
	return lines
}

// shortenPath replaces $HOME prefix with ~ for compact display.
func shortenPath(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(os.PathSeparator))) {
		return "~" + path[len(home):]
	}
	return path
}

// expandPath restores ~ back to the absolute home directory path.
func expandPath(path, home string) string {
	if home != "" && strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

// parseSelectedPath 는 fzf 가 돌려준 선택 줄에서 경로 영역만 떼어 절대 경로로
// 되돌린다. 줄은 "표시\t경로" 형식이므로 마지막 탭 뒤를 경로로 본다. 탭이
// 없으면 줄 전체를 경로로 본다.
func parseSelectedPath(selected, home string) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return ""
	}
	if i := strings.LastIndex(selected, "\t"); i >= 0 {
		selected = selected[i+1:]
	}
	return expandPath(selected, home)
}

// previewCmd builds the fzf preview command, expanding ~ to $HOME for git commands.
// 입력은 "표시\t경로" 두 열이므로 preview 는 경로 영역인 {2} 를 참조한다.
// 수정 시 검토 관점: 입력 열 구성(buildPickerLines·Run 의 입력 포맷)을 바꾸면
// 이 {2} 인덱스도 함께 맞춰야 한다.
func previewCmd(home string) string {
	// fzf 가 {2} 를 작은따옴표로 감싸 치환하므로 여기서 다시 따옴표로 감싸지
	// 않는다. leading ~ 만 home 경로로 치환하되 dash 같은 POSIX sh 에서도
	// 동작하도록 case 와 ${p#~} 만 쓴다. ${p/.../...} 는 bash·zsh 전용이라
	// dash 에서 "Bad substitution" 으로 깨진다.
	resolve := fmt.Sprintf(`p={2}; case "$p" in "~"*) p="%s${p#\~}";; esac`, home)
	return resolve + `; git -C "$p" log --oneline -5 2>/dev/null; echo; echo "branch: $(git -C "$p" branch --show-current 2>/dev/null)"; echo; git -C "$p" status --short 2>/dev/null | head -10`
}
