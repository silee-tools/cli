package pick

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/silee-tools/git-tidy/internal/reason"
)

// RunLine 은 줄 입력으로 다중 선택을 진행하고 체크된 항목을 돌려준다.
// 두 번째 반환값이 false 면 사용자가 취소한 것이다. 표시 정보(신호·worktree·
// 경과)는 Selection 의 Item 이 들고 있다. in/out 은 테스트를 위해 주입한다.
func RunLine(sel *Selection, in io.Reader, out io.Writer) ([]string, bool) {
	r := bufio.NewScanner(in)
	for {
		renderLine(sel, out)
		_, _ = fmt.Fprint(out, "번호=토글, a=전체토글, 빈 줄=완료, q=취소 > ")
		if !r.Scan() {
			return nil, false
		}
		switch cmd := strings.TrimSpace(r.Text()); cmd {
		case "":
			return confirmLine(sel, r, out)
		case "q":
			return nil, false
		case "a":
			sel.ToggleAll()
		default:
			if n, err := strconv.Atoi(cmd); err == nil {
				sel.Toggle(n - 1)
			}
		}
	}
}

func renderLine(sel *Selection, out io.Writer) {
	cur := ""
	for i, it := range sel.Items() {
		if it.Signal != cur {
			cur = it.Signal
			_, _ = fmt.Fprintf(out, "  ── %s (%d) ──\n", cur, sel.GroupCount(cur))
			if desc := reason.Description(cur); desc != "" {
				_, _ = fmt.Fprintf(out, "     %s\n", desc)
			}
		}
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		line := fmt.Sprintf("  %2d. [%s] %s", i+1, mark, it.Name)
		if it.WorktreePath != "" {
			line += "  ⌂ " + filepath.Base(it.WorktreePath)
		}
		if it.AgeDays > 0 {
			line += fmt.Sprintf("   %d일 경과", it.AgeDays)
		}
		if it.Signal == "absorbed" && it.AbsorbedByShortHash != "" {
			line += fmt.Sprintf("   base: %s %s", it.AbsorbedByShortHash, it.AbsorbedBySubject)
		}
		_, _ = fmt.Fprintln(out, line)
	}
}

func confirmLine(sel *Selection, r *bufio.Scanner, out io.Writer) ([]string, bool) {
	checked := sel.Checked()
	if len(checked) == 0 {
		return nil, true
	}
	_, _ = fmt.Fprintf(out, "%d개 브랜치를 삭제합니다. 진행할까요? [y/N] ", len(checked))
	if !r.Scan() {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(r.Text()), "y") {
		return checked, true
	}
	return nil, false
}
