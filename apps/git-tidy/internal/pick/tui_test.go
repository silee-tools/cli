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

func TestTUICursorClampsAtTop(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("커서 0에서 Up 후에도 0 이어야 함: cursor=%d", m.cursor)
	}
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != 0 {
		t.Errorf("커서 0에서 'k' 후에도 0 이어야 함: cursor=%d", m.cursor)
	}
}

func TestTUICursorClampsAtBottom(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	last := len(m.rows) - 1 // rows 5개이므로 4
	for i := 0; i < 10; i++ {
		m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if m.cursor != last {
		t.Errorf("여러 번 'j' 후 마지막 행(%d)에 머물러야 함: cursor=%d", last, m.cursor)
	}
}

func TestTUIWindowFullWhenHeightZero(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	start, end := m.window()
	if start != 0 || end != len(m.rows) {
		t.Errorf("height=0 이면 전체 범위여야 함: (%d,%d), want (0,%d)", start, end, len(m.rows))
	}
}

func TestTUIWindowRespectsSmallHeight(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	// rows 5개보다 visible(=height-4=2) 가 작도록 작은 height 주입.
	m, _ = updateForTest(m, tea.WindowSizeMsg{Width: 80, Height: 6})
	// 커서를 마지막 행으로 옮겨 window 가 끝쪽을 비추게 한다.
	for i := 0; i < 10; i++ {
		m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	start, end := m.window()
	visible := 6 - 4 // height-4
	if end-start > visible {
		t.Errorf("window 폭이 visible(%d) 를 넘음: end-start=%d", visible, end-start)
	}
	if m.cursor < start || m.cursor >= end {
		t.Errorf("window 가 커서(%d)를 포함해야 함: (%d,%d)", m.cursor, start, end)
	}
}
