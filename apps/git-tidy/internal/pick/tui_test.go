package pick

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tuiItems() []Item {
	return []Item{
		{Name: "g1", Signal: "gone", Checked: true},
		{Name: "g2", Signal: "gone", Checked: true},
		{Name: "m1", Signal: "merged", Checked: false},
	}
}

func updateForTest(m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(tuiModel), cmd
}

// rows 는 [헤더 gone, g1, g2, 헤더 merged, m1] 순서다(인덱스 0..4).
func TestTUIRowsLayout(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	if len(m.rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(m.rows))
	}
	if !m.rows[0].isHeader || m.rows[0].signal != "gone" {
		t.Errorf("rows[0] 은 gone 헤더여야 함: %+v", m.rows[0])
	}
	if m.rows[1].isHeader || m.rows[1].itemIdx != 0 {
		t.Errorf("rows[1] 은 g1 항목이어야 함: %+v", m.rows[1])
	}
}

func TestTUIToggleItemOnSpace(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.sel.IsChecked(0) {
		t.Error("space 로 g1 이 해제돼야 함")
	}
}

func TestTUIToggleGroupOnHeader(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.sel.IsChecked(0) || m.sel.IsChecked(1) {
		t.Error("gone 헤더 space 로 그룹 전체가 해제돼야 함")
	}
}

func TestTUIEnterConfirms(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done || m.cancel {
		t.Errorf("enter 는 확정(done=true, cancel=false)이어야 함: done=%v cancel=%v", m.done, m.cancel)
	}
}

func TestTUIEscCancels(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.cancel {
		t.Error("esc 는 취소여야 함")
	}
}
