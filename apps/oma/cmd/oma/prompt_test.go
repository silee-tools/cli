package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/silee-tools/oma/internal/prep"
)

type fakeCandidates struct {
	inputKinds []promptOption
	bases      []promptOption
}

func (f fakeCandidates) InputKinds() []promptOption { return f.inputKinds }
func (f fakeCandidates) Bases() []promptOption      { return f.bases }

type selectCall struct {
	label   string
	options []promptOption
}

type fakePrompter struct {
	selections []string
	inputs     []string
	selectErr  error
	inputErr   error
	confirmed  bool
	confirmErr error
	calls      []selectCall
	confirms   []string
	events     []string
}

func (f *fakePrompter) Select(label string, options []promptOption) (string, error) {
	f.events = append(f.events, "select:"+label)
	f.calls = append(f.calls, selectCall{label: label, options: append([]promptOption(nil), options...)})
	if f.selectErr != nil {
		return "", f.selectErr
	}
	if len(f.selections) == 0 {
		return "", errors.New("unexpected selection")
	}
	value := f.selections[0]
	f.selections = f.selections[1:]
	return value, nil
}

func (f *fakePrompter) Input(label string) (string, error) {
	f.events = append(f.events, "input:"+label)
	if f.inputErr != nil {
		return "", f.inputErr
	}
	if len(f.inputs) == 0 {
		return "", errors.New("unexpected input")
	}
	value := f.inputs[0]
	f.inputs = f.inputs[1:]
	return value, nil
}

func (f *fakePrompter) Confirm(message string) (bool, error) {
	f.events = append(f.events, "confirm:"+message)
	f.confirms = append(f.confirms, message)
	return f.confirmed, f.confirmErr
}

func TestPromptWithExplicitInputAndBaseDoesNotNeedCandidates(t *testing.T) {
	prompt := &fakePrompter{confirmed: true}
	err := run([]string{"prep", "--empty", "--base", "main"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return true },
		Prompter:   prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{"confirm:이 계획을 적용할까요?"}
	if !reflect.DeepEqual(prompt.events, wantEvents) {
		t.Fatalf("events = %v, want %v", prompt.events, wantEvents)
	}
}

func TestPromptCompletesEveryInputKind(t *testing.T) {
	inputKinds := []promptOption{
		{Value: "jira", Label: "Jira 작업"},
		{Value: "description", Label: "작업 설명"},
		{Value: "empty", Label: "빈 작업"},
	}
	tests := []struct {
		name       string
		selection  string
		input      string
		wantInput  prep.Input
		wantEvents []string
	}{
		{
			name:      "jira",
			selection: "jira",
			input:     "abc-123",
			wantInput: prep.Input{Kind: prep.InputJira, IssueKey: "ABC-123", BranchType: "feature", Base: "main", Worktree: "new"},
			wantEvents: []string{
				"select:작업 입력을 선택하세요",
				"input:Jira 키를 입력하세요",
				"confirm:이 계획을 적용할까요?",
			},
		},
		{
			name:      "description",
			selection: "description",
			input:     " 작업 설명 ",
			wantInput: prep.Input{Kind: prep.InputDescription, Description: "작업 설명", BranchType: "feature", Base: "main", Worktree: "new"},
			wantEvents: []string{
				"select:작업 입력을 선택하세요",
				"input:작업 설명을 입력하세요",
				"confirm:이 계획을 적용할까요?",
			},
		},
		{
			name:      "empty",
			selection: "empty",
			wantInput: prep.Input{Kind: prep.InputEmpty, BranchType: "feature", Base: "main", Worktree: "new"},
			wantEvents: []string{
				"select:작업 입력을 선택하세요",
				"confirm:이 계획을 적용할까요?",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := &fakePrompter{selections: []string{tc.selection}, confirmed: true}
			if tc.input != "" {
				prompt.inputs = []string{tc.input}
			}
			parsed := options{Input: prep.Input{BranchType: "feature", Base: "main", Worktree: "new"}}
			err := completeOptions(&parsed, dependencies{
				IsTerminal: func() bool { return true },
				Prompter:   prompt,
				Candidates: fakeCandidates{inputKinds: inputKinds},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed.Input, tc.wantInput) {
				t.Fatalf("input = %+v, want %+v", parsed.Input, tc.wantInput)
			}
			if !reflect.DeepEqual(prompt.events, tc.wantEvents) {
				t.Fatalf("events = %v, want %v", prompt.events, tc.wantEvents)
			}
		})
	}
}

func TestPromptTerminalReaderSequence(t *testing.T) {
	prompt := terminalPrompter{input: strings.NewReader("1\n1\ny\n"), output: &bytes.Buffer{}}
	first, err := prompt.Select("작업 입력", []promptOption{{Value: "empty", Label: "빈 작업"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := prompt.Select("기준 브랜치", []promptOption{{Value: "main", Label: "main"}})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := prompt.Confirm("적용할까요?")
	if err != nil {
		t.Fatal(err)
	}
	if first != "empty" || second != "main" || !approved {
		t.Fatalf("sequence = %q, %q, %t", first, second, approved)
	}
}

func TestPromptUsesInjectedInputAndBaseCandidates(t *testing.T) {
	prompt := &fakePrompter{selections: []string{"empty", "release/1.x"}, confirmed: true}
	candidates := fakeCandidates{
		inputKinds: []promptOption{{Value: "description", Label: "작업 설명"}, {Value: "empty", Label: "빈 작업"}},
		bases:      []promptOption{{Value: "main", Label: "main (origin default)"}, {Value: "release/1.x", Label: "release/1.x"}},
	}
	err := run([]string{"prep"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return true },
		Prompter:   prompt,
		Candidates: candidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.calls) != 2 {
		t.Fatalf("select calls = %d, want 2", len(prompt.calls))
	}
	if !reflect.DeepEqual(prompt.calls[0].options, candidates.inputKinds) || !reflect.DeepEqual(prompt.calls[1].options, candidates.bases) {
		t.Fatalf("prompt options = %+v", prompt.calls)
	}
	if len(prompt.confirms) != 1 {
		t.Fatalf("confirm calls = %d, want 1", len(prompt.confirms))
	}
}

func TestPromptNonTerminalRequiresInputAndBase(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "input", args: []string{"prep", "--base", "main", "--dry-run"}, want: "작업 입력"},
		{name: "base", args: []string{"prep", "--empty", "--dry-run"}, want: "--base"},
		{name: "plan", args: []string{"prep", "--empty", "--base", "main", "--yes"}, want: "--plan"},
		{name: "approval", args: []string{"prep", "--empty", "--base", "main", "--plan", "token"}, want: "--yes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{IsTerminal: func() bool { return false }})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestPromptCancellationAndApprovalRejection(t *testing.T) {
	tests := []struct {
		name   string
		prompt *fakePrompter
	}{
		{name: "selection cancelled", prompt: &fakePrompter{selectErr: errCancelled}},
		{name: "approval rejected", prompt: &fakePrompter{selections: []string{"main"}, confirmed: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := run([]string{"prep", "--empty"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
				IsTerminal: func() bool { return true },
				Prompter:   tc.prompt,
				Candidates: fakeCandidates{bases: []promptOption{{Value: "main", Label: "main"}}},
			})
			if !errors.Is(err, errCancelled) {
				t.Fatalf("run() error = %v, want cancellation", err)
			}
		})
	}
}

func TestPromptPropagatesErrors(t *testing.T) {
	want := errors.New("prompt unavailable")
	prompt := &fakePrompter{selectErr: want}
	err := run([]string{"prep", "--empty"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return true },
		Prompter:   prompt,
		Candidates: fakeCandidates{bases: []promptOption{{Value: "main", Label: "main"}}},
	})
	if !errors.Is(err, want) {
		t.Fatalf("run() error = %v, want %v", err, want)
	}
}
