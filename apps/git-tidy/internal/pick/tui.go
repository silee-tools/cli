package pick

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// RunTUI 는 raw 모드 체크박스 목록으로 다중 선택을 진행한다.
// 두 번째 반환값이 false 면 취소다. raw 모드 진입에 실패하면 ok=false,
// 세 번째 반환값(fellBack)이 true 가 되어 호출자가 줄 기반으로 폴백한다.
func RunTUI(sel *Selection, labels []string) (checked []string, ok bool, fellBack bool) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, false, true
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	cursor := 0
	buf := make([]byte, 3)
	for {
		renderTUI(sel, labels, cursor)
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			continue
		}
		switch {
		case buf[0] == 3 || buf[0] == 27 && n == 1: // Ctrl-C / ESC
			clearTUI(len(sel.Items()))
			return nil, false, false
		case buf[0] == 13: // Enter
			clearTUI(len(sel.Items()))
			return sel.Checked(), true, false
		case buf[0] == ' ':
			sel.Toggle(cursor)
		case buf[0] == 'a':
			sel.ToggleAll()
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 65: // ↑
			if cursor > 0 {
				cursor--
			}
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 66: // ↓
			if cursor < len(sel.Items())-1 {
				cursor++
			}
		}
	}
}

func renderTUI(sel *Selection, labels []string, cursor int) {
	clearTUI(len(sel.Items()))
	for i, item := range sel.Items() {
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		pointer := "  "
		if i == cursor {
			pointer = "> "
		}
		_, _ = fmt.Printf("%s[%s] %s  %s\r\n", pointer, mark, item, labels[i])
	}
	_, _ = fmt.Print("스페이스=토글, a=전체토글, Enter=확정, ESC=취소\r\n")
}

// clearTUI 는 직전 렌더링(목록 줄 + 안내 1줄)을 지워 다시 그릴 자리를 만든다.
func clearTUI(itemCount int) {
	for i := 0; i < itemCount+1; i++ {
		_, _ = fmt.Print("\033[1A\033[2K")
	}
}
