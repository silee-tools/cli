package pick

import (
	"reflect"
	"testing"
)

func items() []Item {
	return []Item{
		{Name: "g1", Signal: "gone", Checked: true},
		{Name: "g2", Signal: "gone", Checked: true},
		{Name: "m1", Signal: "merged", Checked: false},
		{Name: "s1", Signal: "stale", AgeDays: 34, Checked: false},
	}
}

func TestNewSelectionRespectsInitialChecked(t *testing.T) {
	m := NewSelection(items())
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"g1", "g2"}) {
		t.Errorf("초기 체크는 gone 항목만이어야 함, got %v", got)
	}
}

func TestGroupsInOrder(t *testing.T) {
	m := NewSelection(items())
	if got := m.Groups(); !reflect.DeepEqual(got, []string{"gone", "merged", "stale"}) {
		t.Errorf("그룹 순서 mismatch, got %v", got)
	}
}

func TestToggleGroupScopedToSignal(t *testing.T) {
	m := NewSelection(items())
	m.ToggleGroup("merged") // merged 안에 켜진 게 없으므로 전체 체크
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"g1", "g2", "m1"}) {
		t.Errorf("ToggleGroup(merged) 후 %v, want [g1 g2 m1]", got)
	}
	m.ToggleGroup("gone") // gone 둘 다 켜져 있으므로 전체 해제
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"m1"}) {
		t.Errorf("ToggleGroup(gone) 후 %v, want [m1]", got)
	}
}

func TestItemFieldsPreserved(t *testing.T) {
	m := NewSelection(items())
	got := m.Items()
	if got[3].AgeDays != 34 || got[3].Signal != "stale" {
		t.Errorf("Item 필드 보존 실패: %+v", got[3])
	}
}

func TestToggleAllStillWorks(t *testing.T) {
	m := NewSelection(items())
	m.ToggleAll() // 하나라도 켜져 있으면 전체 해제
	if got := m.Checked(); len(got) != 0 {
		t.Errorf("ToggleAll 후 전체 해제 기대, got %v", got)
	}
	m.ToggleAll() // 전부 꺼져 있으면 전체 체크
	if got := m.Checked(); len(got) != 4 {
		t.Errorf("ToggleAll 재호출 후 전체 체크 기대, got %v", got)
	}
}
