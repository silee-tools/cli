package jira

import (
	"strings"
	"testing"
)

func TestSelectTransition(t *testing.T) {
	direct := Transition{ID: "21", Name: "Start", To: Status{Name: "In Progress", CategoryKey: "indeterminate"}}
	newA := Transition{ID: "11", Name: "Refine", To: Status{ID: "2", Name: "Refinement", CategoryKey: "new"}}
	newB := Transition{ID: "12", Name: "Ready", To: Status{ID: "4", Name: "Ready", CategoryKey: "new"}}
	done := Transition{ID: "31", Name: "Close", To: Status{Name: "Done", CategoryKey: "done"}}

	tests := []struct {
		name         string
		current      Status
		available    []Transition
		requestedID  string
		wantID       string
		wantInput    bool
		wantComplete bool
		wantErr      string
	}{
		{name: "direct indeterminate", current: Status{CategoryKey: "new"}, available: []Transition{direct, done}, wantID: "21"},
		{name: "single new intermediate", current: Status{ID: "1", CategoryKey: "new"}, available: []Transition{newA, done}, wantID: "11"},
		{name: "multiple candidates require input", current: Status{ID: "1", CategoryKey: "new"}, available: []Transition{newA, newB, done}, wantInput: true},
		{name: "valid explicit transition", current: Status{CategoryKey: "new"}, available: []Transition{newA, newB}, requestedID: "12", wantID: "12"},
		{name: "invalid explicit transition", current: Status{CategoryKey: "new"}, available: []Transition{newA}, requestedID: "99", wantErr: "not available"},
		{name: "explicit done blocked", current: Status{CategoryKey: "new"}, available: []Transition{done}, requestedID: "31", wantErr: "done"},
		{name: "done current blocked", current: Status{CategoryKey: "done"}, available: []Transition{direct}, wantErr: "done"},
		{name: "already in progress", current: Status{CategoryKey: "indeterminate"}, available: []Transition{done}, wantComplete: true},
		{name: "three hop cap", current: Status{CategoryKey: "new", TransitionCount: 3}, available: []Transition{direct}, wantErr: "3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := SelectTransition(test.current, test.available, test.requestedID)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.wantErr)) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectTransition: %v", err)
			}
			if decision.Complete != test.wantComplete {
				t.Errorf("Complete = %v", decision.Complete)
			}
			if test.wantID != "" {
				if decision.Transition == nil || decision.Transition.ID != test.wantID {
					t.Fatalf("Transition = %#v, want %s", decision.Transition, test.wantID)
				}
			}
			if test.wantInput {
				if len(decision.RequiredInputs) != 1 || decision.RequiredInputs[0].Kind != "transition_id" || len(decision.RequiredInputs[0].Options) != 2 {
					t.Fatalf("RequiredInputs = %#v", decision.RequiredInputs)
				}
			}
		})
	}
}

func TestSelectTransitionRejectsExplicitIDForTerminalCurrentStatus(t *testing.T) {
	available := []Transition{
		{ID: "21", Name: "Start", To: Status{Name: "In Progress", CategoryKey: "indeterminate"}},
		{ID: "31", Name: "Close", To: Status{Name: "Done", CategoryKey: "done"}},
	}
	for _, test := range []struct {
		name      string
		current   Status
		requested string
	}{
		{name: "in progress valid ID", current: Status{CategoryKey: "indeterminate"}, requested: "21"},
		{name: "in progress invalid ID", current: Status{CategoryKey: "indeterminate"}, requested: "99"},
		{name: "done cannot bypass with start ID", current: Status{CategoryKey: "done"}, requested: "21"},
		{name: "done cannot bypass with done ID", current: Status{CategoryKey: "done"}, requested: "31"},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := SelectTransition(test.current, available, test.requested)
			if err == nil {
				t.Fatalf("explicit transition was silently accepted: %#v", decision)
			}
			if decision.Complete || decision.Transition != nil {
				t.Fatalf("terminal state returned an actionable decision: %#v", decision)
			}
		})
	}
}
