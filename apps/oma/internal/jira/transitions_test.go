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
