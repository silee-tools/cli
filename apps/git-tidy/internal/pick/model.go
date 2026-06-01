// Package pick 는 삭제 대상 다중 선택을 담당한다. 순수 선택 모델 위에
// 체크박스 TUI 와 줄 기반 선택 두 front-end 를 둔다.
package pick

// Item 은 선택 대상 하나의 표시 정보와 초기 체크 상태다.
type Item struct {
	Name         string
	Signal       string // 삭제 사유(그룹 키): gone / merged / stale
	WorktreePath string // worktree 에 물려 있으면 그 경로, 아니면 빈 문자열
	AgeDays      int    // stale 경과 일수, 그 외 0
	Checked      bool   // 초기 체크 상태
}

// Selection 은 항목별 체크 상태를 들고 있는 순수 모델이다.
// 렌더링·입력과 분리돼 있어 TUI 와 줄 기반 모드가 함께 쓰고 단위 테스트한다.
type Selection struct {
	items   []Item
	checked []bool
}

// NewSelection 은 각 Item 의 초기 Checked 를 반영한 모델을 만든다.
func NewSelection(items []Item) *Selection {
	checked := make([]bool, len(items))
	for i, it := range items {
		checked[i] = it.Checked
	}
	return &Selection{items: items, checked: checked}
}

// Items 는 전체 항목을 돌려준다.
func (s *Selection) Items() []Item { return s.items }

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

// Groups 는 항목 등장 순서대로 중복 없는 그룹(신호) 키를 돌려준다.
func (s *Selection) Groups() []string {
	var out []string
	seen := map[string]bool{}
	for _, it := range s.items {
		if !seen[it.Signal] {
			seen[it.Signal] = true
			out = append(out, it.Signal)
		}
	}
	return out
}

// ToggleGroup 은 한 그룹 안에 하나라도 체크돼 있으면 그 그룹 전체 해제,
// 아니면 그 그룹 전체 체크한다(ToggleAll 규칙의 그룹 범위판).
func (s *Selection) ToggleGroup(signal string) {
	anyChecked := false
	for i, it := range s.items {
		if it.Signal == signal && s.checked[i] {
			anyChecked = true
			break
		}
	}
	for i, it := range s.items {
		if it.Signal == signal {
			s.checked[i] = !anyChecked
		}
	}
}

// Checked 는 체크된 항목 이름들을 순서대로 돌려준다.
func (s *Selection) Checked() []string {
	var out []string
	for i, c := range s.checked {
		if c {
			out = append(out, s.items[i].Name)
		}
	}
	return out
}
