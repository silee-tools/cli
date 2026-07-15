package jira

import (
	"errors"
	"fmt"
)

const maxTransitionCount = 3

func SelectTransition(current Status, available []Transition, requestedID string) (TransitionDecision, error) {
	switch current.CategoryKey {
	case "indeterminate":
		if requestedID != "" {
			return TransitionDecision{}, errors.New("jira issue is already in progress; explicit transition is not allowed")
		}
		return TransitionDecision{Complete: true}, nil
	case "done":
		return TransitionDecision{}, errors.New("jira issue is in the done category and cannot be started")
	}
	if current.TransitionCount >= maxTransitionCount {
		return TransitionDecision{}, fmt.Errorf("jira transition limit of %d steps reached", maxTransitionCount)
	}
	if requestedID != "" {
		for index := range available {
			if available[index].ID != requestedID {
				continue
			}
			if available[index].To.CategoryKey == "done" {
				return TransitionDecision{}, errors.New("requested Jira transition targets the done category")
			}
			return TransitionDecision{Transition: transitionPointer(available[index])}, nil
		}
		return TransitionDecision{}, fmt.Errorf("requested Jira transition %q is not available", requestedID)
	}

	direct := candidates(available, func(transition Transition) bool {
		return transition.To.CategoryKey == "indeterminate"
	})
	if len(direct) == 1 {
		return TransitionDecision{Transition: transitionPointer(direct[0])}, nil
	}
	if len(direct) > 1 {
		return requireTransitionInput(direct), nil
	}

	intermediate := candidates(available, func(transition Transition) bool {
		return transition.To.CategoryKey == "new" && transition.To.ID != current.ID
	})
	if len(intermediate) == 1 {
		return TransitionDecision{Transition: transitionPointer(intermediate[0])}, nil
	}
	if len(intermediate) > 1 {
		return requireTransitionInput(intermediate), nil
	}
	return TransitionDecision{}, errors.New("no safe Jira transition toward in progress is available")
}

func candidates(available []Transition, include func(Transition) bool) []Transition {
	result := make([]Transition, 0, len(available))
	for _, transition := range available {
		if transition.To.CategoryKey != "done" && include(transition) {
			result = append(result, transition)
		}
	}
	return result
}

func requireTransitionInput(transitions []Transition) TransitionDecision {
	options := make([]TransitionOption, 0, len(transitions))
	for _, transition := range transitions {
		options = append(options, TransitionOption{ID: transition.ID, Status: transition.To.Name})
	}
	return TransitionDecision{RequiredInputs: []RequiredInput{{
		Kind:    "transition_id",
		Message: "Choose a Jira transition toward in progress.",
		Options: options,
	}}}
}

func transitionPointer(transition Transition) *Transition {
	return &transition
}
