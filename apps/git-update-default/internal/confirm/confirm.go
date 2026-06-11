// Package confirm 은 dirty 작업 트리에 대한 3지선다(stash/force/취소)를 받는다.
package confirm

import (
	"os"

	"golang.org/x/term"
)

// Action 은 dirty 일 때 사용자가 고른 처리다.
type Action int

const (
	// ActionCancel 은 아무것도 바꾸지 않고 멈춘다. 기본값(zero value)이다.
	ActionCancel Action = iota
	// ActionStash 는 변경을 stash 한 뒤 진행한다.
	ActionStash
	// ActionForce 는 추적 변경을 버리고 진행한다.
	ActionForce
)

// IsTTY 는 표준 입력이 터미널인지 본다. 아니면 인터랙티브 선택을 띄울 수 없다.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
