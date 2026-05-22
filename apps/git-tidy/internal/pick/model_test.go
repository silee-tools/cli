package pick

import (
	"reflect"
	"testing"
)

func TestSelectionModel(t *testing.T) {
	m := NewSelection([]string{"a", "b", "c"})

	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("초기 상태는 전부 체크여야 함, got %v", got)
	}

	m.Toggle(1) // b 해제
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("Toggle(1) 후 %v, want [a c]", got)
	}

	m.ToggleAll() // 하나라도 켜져 있으면 전체 해제
	if got := m.Checked(); len(got) != 0 {
		t.Errorf("ToggleAll 후 전체 해제 기대, got %v", got)
	}

	m.ToggleAll() // 전부 꺼져 있으면 전체 체크
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("ToggleAll 재호출 후 전체 체크 기대, got %v", got)
	}
}
