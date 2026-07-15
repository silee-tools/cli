package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/silee-tools/oma/internal/prep"
)

type jsonResult struct {
	Status           string              `json:"status"`
	PlanToken        string              `json:"plan_token,omitempty"`
	ExpiresAt        *time.Time          `json:"expires_at"`
	InputKind        prep.InputKind      `json:"input_kind"`
	IssueKey         string              `json:"issue_key,omitempty"`
	Issue            *jsonIssue          `json:"issue,omitempty"`
	JiraSnapshotPath string              `json:"jira_snapshot_path,omitempty"`
	Base             jsonBase            `json:"base"`
	Branch           string              `json:"branch"`
	WorktreePath     string              `json:"worktree_path"`
	Steps            []jsonStep          `json:"steps"`
	Warnings         []string            `json:"warnings"`
	RequiredInputs   []jsonRequiredInput `json:"required_inputs"`
	NextAction       string              `json:"next_action"`
}

type jsonBase struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type jsonStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type jsonRequiredInput struct {
	Kind    string            `json:"kind"`
	Message string            `json:"message"`
	Options []jsonInputOption `json:"options"`
}

type jsonInputOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type jsonIssue struct {
	Key             string `json:"key"`
	Summary         string `json:"summary"`
	DescriptionText string `json:"description_text"`
	Status          string `json:"status"`
	Assignee        string `json:"assignee"`
}

func JSON(writer io.Writer, result prep.Result) error {
	steps := make([]jsonStep, 0, len(result.Steps))
	for _, step := range result.Steps {
		steps = append(steps, jsonStep{Name: step.Name, Status: step.Status, Detail: step.Detail})
	}
	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	required := make([]jsonRequiredInput, 0, len(result.RequiredInputs))
	for _, input := range result.RequiredInputs {
		options := make([]jsonInputOption, 0, len(input.Options))
		for _, option := range input.Options {
			options = append(options, jsonInputOption{Value: option.Value, Label: option.Label})
		}
		required = append(required, jsonRequiredInput{Kind: input.Kind, Message: input.Message, Options: options})
	}
	var expiresAt *time.Time
	if !result.ExpiresAt.IsZero() {
		value := result.ExpiresAt
		expiresAt = &value
	}
	var issue *jsonIssue
	if result.Issue != nil {
		issue = &jsonIssue{
			Key: result.Issue.Key, Summary: result.Issue.Summary,
			DescriptionText: result.Issue.DescriptionText, Status: result.Issue.Status,
			Assignee: result.Issue.Assignee,
		}
	}
	document := jsonResult{
		Status: result.Status, PlanToken: result.PlanToken, ExpiresAt: expiresAt,
		InputKind: result.InputKind, IssueKey: result.IssueKey, Issue: issue,
		JiraSnapshotPath: result.JiraSnapshotPath, Base: jsonBase{Ref: result.Base.Ref, SHA: result.Base.SHA}, Branch: result.Branch,
		WorktreePath: result.WorktreePath, Steps: steps, Warnings: warnings,
		RequiredInputs: required, NextAction: result.NextAction,
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}

func Human(writer io.Writer, result prep.Result) error {
	if _, err := fmt.Fprintf(writer, "상태: %s\n", result.Status); err != nil {
		return err
	}
	if result.Branch != "" {
		if _, err := fmt.Fprintf(writer, "브랜치: %s\n", result.Branch); err != nil {
			return err
		}
	}
	if result.WorktreePath != "" {
		if _, err := fmt.Fprintf(writer, "worktree: %s\n", result.WorktreePath); err != nil {
			return err
		}
	}
	if result.PlanToken != "" {
		if _, err := fmt.Fprintf(writer, "계획 토큰: %s\n", result.PlanToken); err != nil {
			return err
		}
	}
	for _, item := range result.RequiredInputs {
		if _, err := fmt.Fprintf(writer, "필수 입력: %s\n", item.Message); err != nil {
			return err
		}
	}
	if result.NextAction != "" {
		if _, err := fmt.Fprintf(writer, "다음 행동: %s\n", result.NextAction); err != nil {
			return err
		}
	}
	return nil
}
