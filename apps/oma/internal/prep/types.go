package prep

import "time"

type InputKind string

const (
	InputJira        InputKind = "jira"
	InputDescription InputKind = "description"
	InputEmpty       InputKind = "empty"
)

type Input struct {
	Kind         InputKind
	IssueKey     string
	Description  string
	Repo         string
	BranchType   string
	Base         string
	Worktree     string
	ProductType  string
	TransitionID string
	Submodules   []string
	SetupArgs    []string
	NoPush       bool
}

type InputOption struct {
	Value string
	Label string
}

type RequiredInput struct {
	Kind    string
	Message string
	Options []InputOption
}

type Base struct {
	Ref string
	SHA string
}

type Step struct {
	Name   string
	Status string
	Detail string
}

type IssueContext struct {
	Key             string
	Summary         string
	DescriptionText string
	Status          string
	Assignee        string
}

type Result struct {
	Status           string
	PlanToken        string
	ExpiresAt        time.Time
	InputKind        InputKind
	IssueKey         string
	Issue            *IssueContext
	JiraSnapshotPath string
	Branch           string
	WorktreePath     string
	NextAction       string
	Base             Base
	Steps            []Step
	Warnings         []string
	RequiredInputs   []RequiredInput
}

type Names struct {
	Branch   string
	Worktree string
}
