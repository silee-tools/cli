package pick

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RunLine 은 줄 입력으로 다중 선택을 진행하고 체크된 항목을 돌려준다.
// 두 번째 반환값이 false 면 사용자가 취소한 것이다.
// labels 는 항목마다 옆에 붙일 부가 설명(신호 등)이다. in/out 은 테스트를
// 위해 주입한다.
func RunLine(sel *Selection, labels []string, in io.Reader, out io.Writer) ([]string, bool) {
	r := bufio.NewScanner(in)
	for {
		renderLine(sel, labels, out)
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

func renderLine(sel *Selection, labels []string, out io.Writer) {
	for i, item := range sel.Items() {
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		_, _ = fmt.Fprintf(out, "  %2d. [%s] %s  %s\n", i+1, mark, item, labels[i])
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
