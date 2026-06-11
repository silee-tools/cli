package confirm

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// choice 는 선택 한 줄이다.
type choice struct {
	action Action
	key    rune   // 단축키
	label  string // 표시 문구
}

// choices 의 순서가 화면 표시 순서이자 커서 인덱스다. 표시 순서는 stash → force →
// 취소이며, 초기 커서는 마지막(취소)에 둔다. 되돌릴 수 없는 force 의 오선택을
// 줄이기 위함이다 — 이 순서와 초기 커서를 바꾸면 안전 기본값이 깨진다.
var choices = []choice{
	{ActionStash, 's', "stash 후 진행 — 변경을 보관하고 default branch 로 전환"},
	{ActionForce, 'f', "force — 추적 변경을 버리고 진행 (되돌릴 수 없음)"},
	{ActionCancel, 'c', "취소 — 아무것도 바꾸지 않고 멈춤"},
}

type model struct {
	files  []string
	cursor int
	chosen Action
	done   bool
}

func newModel(files []string) model {
	return model{files: files, cursor: len(choices) - 1, chosen: ActionCancel}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case k.Type == tea.KeyCtrlC || k.Type == tea.KeyEsc:
			m.chosen, m.done = ActionCancel, true
			return m, tea.Quit
		case k.Type == tea.KeyEnter:
			m.chosen, m.done = choices[m.cursor].action, true
			return m, tea.Quit
		case k.Type == tea.KeyUp:
			m.move(-1)
		case k.Type == tea.KeyDown:
			m.move(1)
		case k.Type == tea.KeyRunes && len(k.Runes) == 1:
			switch k.Runes[0] {
			case 'k':
				m.move(-1)
			case 'j':
				m.move(1)
			default:
				for _, c := range choices {
					if c.key == k.Runes[0] {
						m.chosen, m.done = c.action, true
						return m, tea.Quit
					}
				}
			}
		}
	}
	return m, nil
}

func (m *model) move(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(choices)-1 {
		m.cursor = len(choices) - 1
	}
}

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleHelp  = lipgloss.NewStyle().Faint(true)
	styleDim   = lipgloss.NewStyle().Faint(true)
	styleCur   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
)

func (m model) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(
		fmt.Sprintf("커밋되지 않은 변경 %d개 — 어떻게 할까요?", len(m.files))) + "\n\n")
	for _, f := range m.files {
		b.WriteString(styleDim.Render("  "+f) + "\n")
	}
	b.WriteString("\n")
	for i, c := range choices {
		line := fmt.Sprintf("[%c] %s", c.key, c.label)
		if i == m.cursor {
			b.WriteString(styleCur.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + styleHelp.Render("↑↓/jk 이동 · enter 선택 · s/f/c 바로가기 · esc 취소"))
	return b.String()
}

// Run 은 bubbletea 단일 선택 화면을 띄워 Action 을 돌려준다. TTY 가 아니어서
// 프로그램 시작에 실패하면 ActionCancel 을 돌려준다(호출자는 IsTTY 로 미리 거른다).
func Run(files []string) Action {
	final, err := tea.NewProgram(newModel(files)).Run()
	if err != nil {
		return ActionCancel
	}
	fm, _ := final.(model)
	return fm.chosen
}
