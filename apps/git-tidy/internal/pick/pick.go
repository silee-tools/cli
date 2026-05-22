package pick

import (
	"os"

	"golang.org/x/term"
)

// Mode 는 다중 선택 방식이다.
type Mode int

const (
	ModeTUI  Mode = iota // 체크박스 TUI
	ModeLine             // 줄 기반 선택
	ModeNone             // 입력 불가(터미널 아님)
)

// DetectMode 는 실행 환경을 보고 선택 방식을 고른다.
// forceLine 이 참이면 터미널인 한 항상 ModeLine 을 돌려준다.
func DetectMode(forceLine bool) Mode {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ModeNone
	}
	if forceLine {
		return ModeLine
	}
	if t := os.Getenv("TERM"); t == "" || t == "dumb" {
		return ModeLine
	}
	return ModeTUI
}
