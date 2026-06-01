package pick

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// row 는 화면 한 줄이다. 그룹 헤더이거나 항목이다.
type row struct {
	isHeader bool
	signal   string // 헤더면 그룹 키, 항목이면 그 항목의 신호
	itemIdx  int    // 항목이면 sel.items 인덱스, 헤더면 -1
}

type tuiModel struct {
	sel    *Selection
	rows   []row
	cursor int
	height int // 화면 높이(WindowSizeMsg). 0이면 전체 렌더.
	done   bool
	cancel bool
}

func buildRows(sel *Selection) []row {
	var rows []row
	cur := ""
	for i, it := range sel.Items() {
		if it.Signal != cur {
			cur = it.Signal
			rows = append(rows, row{isHeader: true, signal: cur, itemIdx: -1})
		}
		rows = append(rows, row{signal: it.Signal, itemIdx: i})
	}
	return rows
}

func newTUIModel(sel *Selection) tuiModel {
	return tuiModel{sel: sel, rows: buildRows(sel), cursor: 0}
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
			m.cancel, m.done = true, true
			return m, tea.Quit
		case msg.Type == tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case msg.Type == tea.KeyUp:
			m.moveCursor(-1)
		case msg.Type == tea.KeyDown:
			m.moveCursor(1)
		case msg.Type == tea.KeySpace:
			m.toggleAtCursor()
		case msg.Type == tea.KeyRunes && len(msg.Runes) == 1:
			switch msg.Runes[0] {
			case 'k':
				m.moveCursor(-1)
			case 'j':
				m.moveCursor(1)
			case 'a':
				m.sel.ToggleAll()
			case 'q':
				m.cancel, m.done = true, true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *tuiModel) moveCursor(delta int) {
	n := len(m.rows)
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > n-1 {
		m.cursor = n - 1
	}
}

func (m *tuiModel) toggleAtCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isHeader {
		m.sel.ToggleGroup(r.signal)
	} else {
		m.sel.Toggle(r.itemIdx)
	}
}

var (
	styleTitle   = lipgloss.NewStyle().Bold(true)
	styleHelp    = lipgloss.NewStyle().Faint(true)
	styleDim     = lipgloss.NewStyle().Faint(true)
	styleCursor  = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerColors = map[string]lipgloss.Style{
		"gone":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
		"merged": lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
		"stale":  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
	}
	headerHint = map[string]string{
		"gone":   "← upstream 사라짐 · PR 머지 후",
		"merged": "← base 에 이미 합쳐짐",
		"stale":  "← 오래 경과",
	}
)

func headerStyle(signal string) lipgloss.Style {
	if s, ok := headerColors[signal]; ok {
		return s
	}
	return lipgloss.NewStyle().Bold(true)
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(
		fmt.Sprintf("git-tidy — 삭제할 브랜치 선택  (%d/%d 선택됨)",
			len(m.sel.Checked()), len(m.sel.Items()))) + "\n\n")

	start, end := m.window()
	for idx := start; idx < end; idx++ {
		b.WriteString(m.renderRow(idx) + "\n")
	}
	b.WriteString("\n" + styleHelp.Render(
		"↑↓/jk 이동 · space 토글 · a 전체 · enter 삭제 · esc 취소"))
	return b.String()
}

// window 는 화면 높이에 맞춰 그릴 row 범위를 돌려준다. height 가 0이면 전체.
func (m tuiModel) window() (int, int) {
	n := len(m.rows)
	visible := m.height - 4 // 제목 2줄 + 안내 2줄
	if m.height == 0 || visible >= n || visible < 1 {
		return 0, n
	}
	start := m.cursor - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > n {
		end = n
		start = end - visible
	}
	return start, end
}

func (m tuiModel) renderRow(idx int) string {
	r := m.rows[idx]
	cursor := "  "
	if idx == m.cursor {
		cursor = "› "
	}
	if r.isHeader {
		count := m.sel.GroupCount(r.signal)
		head := fmt.Sprintf("▾ %s (%d)", r.signal, count)
		line := cursor + headerStyle(r.signal).Render(head) + "  " + styleDim.Render(headerHint[r.signal])
		if idx == m.cursor {
			return styleCursor.Render(strings.TrimRight(line, " "))
		}
		return line
	}
	it := m.sel.Items()[r.itemIdx]
	box := "◯"
	if m.sel.IsChecked(r.itemIdx) {
		box = styleChecked.Render("◉")
	}
	line := fmt.Sprintf("%s  %s %s", cursor, box, it.Name)
	if it.WorktreePath != "" {
		line += styleDim.Render("   ⌂ "+filepath.Base(it.WorktreePath)) + styleDim.Render("  [worktree 동반 제거]")
	}
	if it.AgeDays > 0 {
		line += styleDim.Render(fmt.Sprintf("   %d일 경과", it.AgeDays))
	}
	if idx == m.cursor {
		return styleCursor.Render(line)
	}
	return line
}

// RunTUI 는 bubbletea 체크박스 목록으로 다중 선택을 진행한다.
// 두 번째 반환값이 false 면 취소다. TTY 가 아니어서 프로그램 시작에 실패하면
// ok=false, 세 번째 반환값(fellBack)이 true 가 되어 호출자가 줄 기반으로 폴백한다.
func RunTUI(sel *Selection) (checked []string, ok bool, fellBack bool) {
	m := newTUIModel(sel)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, false, true
	}
	fm, _ := final.(tuiModel)
	if fm.cancel {
		return nil, false, false
	}
	return sel.Checked(), true, false
}
