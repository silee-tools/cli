package confirm

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestModelDefaultsToCancel(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	// 초기 커서가 취소이므로 enter 를 바로 누르면 취소가 선택된다.
	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := got.(model)
	if !fm.done || fm.chosen != ActionCancel {
		t.Fatalf("default enter -> chosen=%v done=%v, want ActionCancel,true", fm.chosen, fm.done)
	}
}

func TestModelStashShortcut(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	got, _ := m.Update(keyRune('s'))
	fm := got.(model)
	if !fm.done || fm.chosen != ActionStash {
		t.Fatalf("'s' -> chosen=%v done=%v, want ActionStash,true", fm.chosen, fm.done)
	}
}

func TestModelForceShortcut(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	got, _ := m.Update(keyRune('f'))
	fm := got.(model)
	if !fm.done || fm.chosen != ActionForce {
		t.Fatalf("'f' -> chosen=%v done=%v, want ActionForce,true", fm.chosen, fm.done)
	}
}

func TestModelArrowToStashThenEnter(t *testing.T) {
	// 커서 순서: stash(0) / force(1) / 취소(2). 초기 커서는 취소(2).
	m := newModel([]string{" M f.txt"})
	up1, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})           // 2 -> 1 (force)
	up2, _ := up1.(model).Update(tea.KeyMsg{Type: tea.KeyUp}) // 1 -> 0 (stash)
	got, _ := up2.(model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := got.(model)
	if fm.chosen != ActionStash {
		t.Fatalf("up up enter -> chosen=%v, want ActionStash", fm.chosen)
	}
}

func TestModelEscCancels(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	// 커서를 force 로 옮겨도 esc 는 취소다.
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got, _ := up.(model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	fm := got.(model)
	if !fm.done || fm.chosen != ActionCancel {
		t.Fatalf("esc -> chosen=%v done=%v, want ActionCancel,true", fm.chosen, fm.done)
	}
}
