package prep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/jira"
	"github.com/silee-tools/oma/internal/state"
)

func TestApplyDescriptionOrdersGitAndNeverTouchesJira(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputDescription, Description: "작업", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events = nil

	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
	want := []string{"fetch", "inspect", "worktree", "submodules", "push"}
	if !reflect.DeepEqual(git.events, want) {
		t.Fatalf("events = %v, want %v", git.events, want)
	}
	if store.consumes != 1 {
		t.Fatalf("consume calls = %d", store.consumes)
	}
}

func TestApplyPushFailureStopsBeforeJiraAndReturnsPartial(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot(), failAt: "push"}
	store := &fakePlanStore{}
	j := &fakeJiraGateway{issue: jiraIssueInProgress()}
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig()}, fakeJiraProvider{gateway: j})
	planned, err := planner.build(context.Background(), Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events, j.events = nil, nil

	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" {
		t.Fatalf("status = %q, want partial", result.Status)
	}
	for _, event := range j.events {
		if event == "fields" || event == "transition" {
			t.Fatalf("Jira write after push failure: %v", j.events)
		}
	}
}

func jiraIssueInProgress() jira.Issue {
	return jira.Issue{
		Key: "ABC-123", Summary: "작업",
		Status: jira.Status{ID: "2", Name: "진행 중", CategoryKey: "indeterminate"},
		CustomFields: map[string]json.RawMessage{
			"custom_product": json.RawMessage(`{"value":"Feature"}`),
			"custom_start":   json.RawMessage(`"2026-07-14"`),
		},
	}
}

func TestApplyDriftConsumesOldTokenAndReturnsNewPlan(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.snapshot.BaseSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	result, err := planner.Apply(context.Background(), "old-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken == "" {
		t.Fatalf("result = %+v", result)
	}
	if store.consumes != 1 || store.creates != 1 {
		t.Fatalf("consume=%d create=%d", store.consumes, store.creates)
	}
	if git.writes != 0 {
		t.Fatalf("Git writes = %d", git.writes)
	}
}

func TestApplyJiraMigrationPrecedesGitAndJiraWritesFollowPush(t *testing.T) {
	migration := &fakeMigration{}
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	git.beforeWrite = func() {
		if migration.applied != 1 {
			t.Fatalf("Git write ran before migration: applied=%d", migration.applied)
		}
	}
	store := &fakePlanStore{}
	j := &fakeJiraGateway{issue: jiraIssueInProgress()}
	j.beforeWrite = func() {
		if !slices.Contains(git.events, "push") {
			t.Fatalf("Jira write ran before push: %v", git.events)
		}
	}
	j.issue.Assignee = nil
	j.issue.CustomFields["custom_start"] = json.RawMessage("null")
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig(), migration: migration}, fakeJiraProvider{gateway: j})
	planned, err := planner.build(context.Background(), Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events, j.events = nil, nil

	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
	if !slices.Contains(git.events, "push") || !slices.Contains(j.events, "fields") {
		t.Fatalf("git=%v jira=%v", git.events, j.events)
	}
	if migration.applied != 1 {
		t.Fatalf("migration applies = %d", migration.applied)
	}
}

func TestApplyWorktreeFailureIsFailedWithoutLaterWrites(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot(), failAt: "worktree"}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events = nil
	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || !reflect.DeepEqual(git.events, []string{"fetch", "inspect", "worktree"}) {
		t.Fatalf("result=%+v events=%v", result, git.events)
	}
}

func TestApplyWithoutOriginSkipsPushAndCompletes(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot(), noOrigin: true}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events = nil
	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || slices.Contains(git.events, "push") {
		t.Fatalf("result=%+v events=%v", result, git.events)
	}
}

func TestApplyRetrySkipsSetupAfterDurableReceipt(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot(), failAt: "push"}
	git.snapshot.SetupHash = "setup-hash"
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	input := Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new", SetupArgs: []string{"--mode", "agent"}}
	planned, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	first, err := planner.Apply(context.Background(), "first-token")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "partial" || countEvent(git.events, "setup") != 1 || store.receiptCreates != 1 {
		t.Fatalf("first=%+v events=%v receiptCreates=%d", first, git.events, store.receiptCreates)
	}
	git.failAt = ""
	git.events = nil
	second, err := planner.Apply(context.Background(), "second-token")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "completed" || countEvent(git.events, "setup") != 0 || store.receiptCreates != 1 {
		t.Fatalf("second=%+v events=%v receiptCreates=%d", second, git.events, store.receiptCreates)
	}
}

func TestApplyStopsBeforePushWhenSetupReceiptIsNotDurable(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	git.snapshot.SetupHash = "setup-hash"
	store := &fakePlanStore{receiptCreateErr: errors.New("injected receipt durability failure")}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || slices.Contains(git.events, "push") || store.receiptCreates != 1 {
		t.Fatalf("result=%+v events=%v receiptCreates=%d", result, git.events, store.receiptCreates)
	}
}

func TestApplyMissingSetupScriptDoesNotWriteReceipt(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	if _, err := planner.Apply(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if store.receiptChecks != 0 || store.receiptCreates != 0 || countEvent(git.events, "setup") != 0 {
		t.Fatalf("receiptChecks=%d creates=%d events=%v", store.receiptChecks, store.receiptCreates, git.events)
	}
}

func TestApplyTreatsCommittedConsumeAsCompleted(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{consumeErr: &state.CommittedError{
		Token: "token",
		State: state.Consumed,
		Err:   errors.New("injected post-publication failure"),
	}}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}

	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatalf("Apply returned committed consume error: %v", err)
	}
	if result.Status != "completed" || len(result.Warnings) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplyWarnsForAmbiguousCommittedConsume(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{consumeErr: &state.CommittedError{
		Token:     "token",
		State:     state.Consumed,
		Ambiguous: true,
		Err:       errors.New("injected ambiguous post-publication failure"),
	}}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}

	result, err := planner.Apply(context.Background(), "token")
	if err != nil {
		t.Fatalf("Apply returned ambiguous committed consume error: %v", err)
	}
	if result.Status != "completed" || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplyPropagatesUncommittedConsumeFailure(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{consumeErr: errors.New("injected consume failure")}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	planned, err := planner.build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}

	result, err := planner.Apply(context.Background(), "token")
	if err == nil || !errors.Is(err, store.consumeErr) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
}

func countEvent(events []string, target string) int {
	count := 0
	for _, event := range events {
		if event == target {
			count++
		}
	}
	return count
}

func TestApplyJiraAllowsThirdTransitionToReachInProgress(t *testing.T) {
	gateway := &threeHopJira{}
	payload := planPayload{
		Input:      Input{TransitionID: "step-1"},
		Issue:      &plannedIssue{Key: "ABC-123"},
		JiraConfig: &config.Config{},
	}
	if err := applyJira(context.Background(), gateway, payload, time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	if gateway.applied != 3 {
		t.Fatalf("applied transitions = %d, want 3", gateway.applied)
	}
}

func TestApplyTransitionOnlyDriftConsumesOldTokenAndCreatesNewPlan(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	j := &fakeJiraGateway{
		issue: jira.Issue{
			Key: "ABC-123", Summary: "작업",
			Status: jira.Status{ID: "1", Name: "할 일", CategoryKey: "new"},
			CustomFields: map[string]json.RawMessage{
				"custom_product": json.RawMessage(`{"value":"Feature"}`),
				"custom_start":   json.RawMessage(`"2026-07-14"`),
			},
		},
		transitions: []jira.Transition{{ID: "21", Name: "Start", To: jira.Status{ID: "2", Name: "진행 중", CategoryKey: "indeterminate"}}},
	}
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig()}, fakeJiraProvider{gateway: j})
	input := Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}
	planned, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	j.transitions = []jira.Transition{{ID: "31", Name: "Start changed", To: jira.Status{ID: "2", Name: "진행 중", CategoryKey: "indeterminate"}}}
	git.writes = 0
	result, err := planner.Apply(context.Background(), "old-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken == "" {
		t.Fatalf("result = %+v", result)
	}
	if git.writes != 0 || store.consumes != 1 || store.creates != 1 {
		t.Fatalf("writes=%d consumes=%d creates=%d", git.writes, store.consumes, store.creates)
	}
}

type threeHopJira struct{ applied int }

func (g *threeHopJira) FetchIssue(context.Context, string) (jira.Issue, []byte, error) {
	category := "new"
	if g.applied == 3 {
		category = "indeterminate"
	}
	return jira.Issue{Key: "ABC-123", Status: jira.Status{ID: fmt.Sprint(g.applied), Name: category, CategoryKey: category}, CustomFields: map[string]json.RawMessage{}}, nil, nil
}
func (*threeHopJira) Myself(context.Context) (jira.User, error) { return jira.User{}, nil }
func (g *threeHopJira) Transitions(context.Context, string) ([]jira.Transition, error) {
	category := "new"
	if g.applied == 2 {
		category = "indeterminate"
	}
	return []jira.Transition{{ID: fmt.Sprintf("step-%d", g.applied+1), To: jira.Status{ID: fmt.Sprint(g.applied + 1), Name: category, CategoryKey: category}}}, nil
}
func (*threeHopJira) UpdateFields(context.Context, string, map[string]any) error { return nil }
func (g *threeHopJira) ApplyTransition(context.Context, string, string) error {
	g.applied++
	return nil
}
func (*threeHopJira) WriteSnapshot(string, []byte) error { return nil }

type fakeMigration struct {
	applied int
	events  *[]string
}

func (f *fakeMigration) Apply(validate func(config.Config) error) error {
	f.applied++
	if f.events != nil {
		*f.events = append(*f.events, "migration")
	}
	return validate(testConfig())
}
