package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
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
	selectErr  error
	confirmed  bool
	confirmErr error
	calls      []selectCall
	confirms   []string
}

func (f *fakePrompter) Select(label string, options []promptOption) (string, error) {
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

func (f *fakePrompter) Confirm(message string) (bool, error) {
	f.confirms = append(f.confirms, message)
	return f.confirmed, f.confirmErr
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
