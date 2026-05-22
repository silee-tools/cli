// Package pick 는 삭제 대상 다중 선택을 담당한다. 순수 선택 모델 위에
// 체크박스 TUI 와 줄 기반 선택 두 front-end 를 둔다.
package pick

// Selection 은 항목별 체크 상태를 들고 있는 순수 모델이다.
// 렌더링·입력과 분리돼 있어 TUI 와 줄 기반 모드가 함께 쓰고 단위 테스트한다.
type Selection struct {
	items   []string
	checked []bool
}

// NewSelection 은 모든 항목이 체크된 상태의 모델을 만든다.
func NewSelection(items []string) *Selection {
	checked := make([]bool, len(items))
	for i := range checked {
		checked[i] = true
	}
	return &Selection{items: items, checked: checked}
}

// Items 는 전체 항목을 돌려준다.
func (s *Selection) Items() []string { return s.items }

// IsChecked 는 i 번째 항목의 체크 여부다.
func (s *Selection) IsChecked(i int) bool { return s.checked[i] }

// Toggle 은 i 번째 항목의 체크를 뒤집는다.
func (s *Selection) Toggle(i int) {
	if i >= 0 && i < len(s.checked) {
		s.checked[i] = !s.checked[i]
	}
}

// ToggleAll 은 하나라도 체크돼 있으면 전체 해제, 아니면 전체 체크한다.
func (s *Selection) ToggleAll() {
	anyChecked := false
	for _, c := range s.checked {
		if c {
			anyChecked = true
			break
		}
	}
	for i := range s.checked {
		s.checked[i] = !anyChecked
	}
}

// Checked 는 체크된 항목들을 순서대로 돌려준다.
func (s *Selection) Checked() []string {
	var out []string
	for i, c := range s.checked {
		if c {
			out = append(out, s.items[i])
		}
	}
	return out
}
