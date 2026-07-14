package prep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/gitops"
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

func TestApplyConfigurationMigrationDriftReturnsFreshPlanBeforeWrites(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	j := &fakeJiraGateway{issue: jiraIssueInProgress()}
	configs := &mutableConfigGateway{config: testConfig()}
	planner := testPlanner(store, git, configs, fakeJiraProvider{gateway: j})
	input := Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}
	planned, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	migration := &fakeMigration{}
	configs.migration = migration
	git.events, j.events = nil, nil

	result, err := planner.Apply(context.Background(), "old-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken == "" || result.PlanToken == "old-token" || store.creates != 1 || store.consumes != 1 {
		t.Fatalf("result=%+v creates=%d consumes=%d", result, store.creates, store.consumes)
	}
	if migration.applied != 0 || git.writes != 0 || slices.Contains(j.events, "fields") || slices.Contains(j.events, "transition") {
		t.Fatalf("migration=%d gitWrites=%d jira=%v", migration.applied, git.writes, j.events)
	}
}

func TestApplyConfigurationMigrationApplyRaceReturnsFreshPlanBeforeExternalWrites(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	j := &fakeJiraGateway{issue: jiraIssueInProgress()}
	migration := &fakeMigration{applyErr: config.ErrMigrationStateChanged}
	configs := fakeConfigGateway{config: testConfig(), migration: migration}
	planner := testPlanner(store, git, configs, fakeJiraProvider{gateway: j})
	input := Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}
	planned, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	git.events, j.events = nil, nil

	result, err := planner.Apply(context.Background(), "old-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken == "" || result.PlanToken == "old-token" || store.creates != 1 || store.consumes != 1 {
		t.Fatalf("result=%+v creates=%d consumes=%d", result, store.creates, store.consumes)
	}
	if git.writes != 0 || slices.Contains(j.events, "fields") || slices.Contains(j.events, "transition") {
		t.Fatalf("gitWrites=%d jira=%v", git.writes, j.events)
	}
}

func TestApplyExpiredPlanReturnsFreshPlanBeforeExternalWrites(t *testing.T) {
	for _, test := range []struct {
		name  string
		input Input
		jira  bool
	}{
		{name: "description", input: Input{Kind: InputDescription, Description: "작업", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}},
		{name: "empty", input: Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}},
		{name: "jira", input: Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}, jira: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			git := &fakeGitGateway{snapshot: testGitSnapshot()}
			store := &fakePlanStore{}
			var migrationPlans int
			configs := configGateway(panicConfigGateway{})
			provider := jiraProvider(panicJiraProvider{})
			var jiraFake *fakeJiraGateway
			if test.jira {
				jiraFake = &fakeJiraGateway{issue: jiraIssueInProgress()}
				configs = fakeConfigGateway{config: testConfig(), planMigrationCalls: &migrationPlans}
				provider = fakeJiraProvider{gateway: jiraFake}
			}
			planner := testPlanner(store, git, configs, provider)
			planned, err := planner.build(context.Background(), test.input)
			if err != nil {
				t.Fatal(err)
			}
			store.claimed = planned.payload
			store.claimRecord = state.Record{Token: "old-expired-token", Fingerprint: planned.fingerprint, CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(-30 * time.Minute), State: state.Pending}
			store.claimErr = state.ErrExpired
			git.events = nil
			if jiraFake != nil {
				jiraFake.events = nil
			}

			result, err := planner.Apply(context.Background(), "old-expired-token")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "planned" || result.PlanToken == "" || result.PlanToken == "old-expired-token" || result.ExpiresAt.IsZero() || !strings.Contains(result.NextAction, "만료") {
				t.Fatalf("result = %+v", result)
			}
			if store.creates != 1 || store.consumes != 0 || git.writes != 0 || migrationPlans != 0 {
				t.Fatalf("creates=%d consumes=%d gitWrites=%d migrationPlans=%d", store.creates, store.consumes, git.writes, migrationPlans)
			}
			if jiraFake != nil && (slices.Contains(jiraFake.events, "fields") || slices.Contains(jiraFake.events, "transition")) {
				t.Fatalf("expired refresh wrote Jira: %v", jiraFake.events)
			}
		})
	}
}

func TestApplyExpiredPlanReturnsRequiredInputsWithoutFreshToken(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	store := &fakePlanStore{}
	issue := jiraIssueInProgress()
	issue.CustomFields["custom_product"] = json.RawMessage("null")
	jiraFake := &fakeJiraGateway{issue: issue}
	var migrationPlans int
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig(), planMigrationCalls: &migrationPlans}, fakeJiraProvider{gateway: jiraFake})
	input := Input{Kind: InputJira, IssueKey: "ABC-123", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}
	planned, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Token: "old-expired-token", Fingerprint: planned.fingerprint, State: state.Pending}
	store.claimErr = state.ErrExpired
	git.events, jiraFake.events = nil, nil

	result, err := planner.Apply(context.Background(), "old-expired-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken != "" || len(result.RequiredInputs) != 1 || result.RequiredInputs[0].Kind != "product_type" {
		t.Fatalf("result = %+v", result)
	}
	if store.creates != 0 || store.consumes != 0 || git.writes != 0 || migrationPlans != 0 {
		t.Fatalf("creates=%d consumes=%d gitWrites=%d migrationPlans=%d", store.creates, store.consumes, git.writes, migrationPlans)
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

func TestSetupReceiptKeyChangesWithSelectedSubmoduleState(t *testing.T) {
	planner := testPlanner(&fakePlanStore{}, &fakeGitGateway{}, panicConfigGateway{}, panicJiraProvider{})
	payload := planPayload{
		CommonDir:    "/repo/.git",
		WorktreePath: "/repo/.worktrees/work",
		Branch:       "feature/work",
		Base:         Base{SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Git: gitops.Snapshot{
			SetupHash:  "setup-hash",
			Submodules: []gitops.Submodule{{Path: "modules/a", URL: "https://example.test/a.git", BaseRef: "main", BaseSHA: "1111111111111111111111111111111111111111"}},
		},
	}
	original, err := planner.setupReceiptKey(payload)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*gitops.Submodule){
		"path":     func(value *gitops.Submodule) { value.Path = "modules/b" },
		"url":      func(value *gitops.Submodule) { value.URL = "https://example.test/other.git" },
		"base ref": func(value *gitops.Submodule) { value.BaseRef = "release" },
		"base sha": func(value *gitops.Submodule) { value.BaseSHA = "2222222222222222222222222222222222222222" },
	}
	for name, change := range changes {
		changed := payload
		changed.Git.Submodules = append([]gitops.Submodule(nil), payload.Git.Submodules...)
		change(&changed.Git.Submodules[0])
		key, err := planner.setupReceiptKey(changed)
		if err != nil {
			t.Fatal(err)
		}
		if key == original {
			t.Fatalf("%s did not change setup receipt key %s", name, original)
		}
	}
}

func TestSetupReceiptKeyNormalizesSubmoduleOrder(t *testing.T) {
	planner := testPlanner(&fakePlanStore{}, &fakeGitGateway{}, panicConfigGateway{}, panicJiraProvider{})
	payload := planPayload{
		CommonDir: "/repo/.git", WorktreePath: "/repo/.worktrees/work", Branch: "feature/work",
		Base: Base{SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Git: gitops.Snapshot{SetupHash: "setup-hash",
			Submodules: []gitops.Submodule{
				{Path: "modules/b", URL: "https://example.test/b.git", BaseRef: "main", BaseSHA: "2222222222222222222222222222222222222222"},
				{Path: "modules/a", URL: "https://example.test/a.git", BaseRef: "main", BaseSHA: "1111111111111111111111111111111111111111"},
			}},
	}
	first, err := planner.setupReceiptKey(payload)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(payload.Git.Submodules)
	second, err := planner.setupReceiptKey(payload)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("submodule order changed receipt key: %s != %s", first, second)
	}
}

func TestApplyRerunsSetupWhenSelectedSubmoduleStateChanges(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot(), failAt: "push"}
	git.snapshot.SetupHash = "setup-hash"
	git.snapshot.Submodules = []gitops.Submodule{{Path: "modules/a", URL: "https://example.test/a.git", BaseRef: "main", BaseSHA: "1111111111111111111111111111111111111111"}}
	store := &fakePlanStore{}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	input := Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new", Submodules: []string{"modules/a"}}
	states := []struct{ path, sha string }{
		{"modules/a", "1111111111111111111111111111111111111111"},
		{"modules/a", "2222222222222222222222222222222222222222"},
		{"modules/b", "2222222222222222222222222222222222222222"},
	}
	for index, submodule := range states {
		git.snapshot.Submodules[0].Path = submodule.path
		git.snapshot.Submodules[0].BaseSHA = submodule.sha
		input.Submodules = []string{submodule.path}
		planned, err := planner.build(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		store.claimed = planned.payload
		store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
		result, err := planner.Apply(context.Background(), fmt.Sprintf("token-%d", index))
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "partial" {
			t.Fatalf("apply %d result = %+v", index, result)
		}
	}
	if countEvent(git.events, "setup") != 3 || store.receiptCreates != 3 {
		t.Fatalf("events=%v receiptCreates=%d", git.events, store.receiptCreates)
	}
}

func TestConcurrentPlannerApplyRunsSetupOnceAndReusesDurableReceipt(t *testing.T) {
	store := &fakePlanStore{}
	gits := []*fakeGitGateway{{snapshot: testGitSnapshot()}, {snapshot: testGitSnapshot()}}
	for _, git := range gits {
		git.snapshot.SetupHash = "setup-hash"
	}
	planners := []*Planner{
		testPlanner(store, gits[0], panicConfigGateway{}, panicJiraProvider{}),
		testPlanner(store, gits[1], panicConfigGateway{}, panicJiraProvider{}),
	}
	planned, err := planners[0].build(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	for _, git := range gits {
		git.events = nil
	}
	start := make(chan struct{})
	results := make(chan Result, 2)
	errorsFound := make(chan error, 2)
	var wait sync.WaitGroup
	for index, planner := range planners {
		wait.Add(1)
		go func(index int, planner *Planner) {
			defer wait.Done()
			<-start
			result, applyErr := planner.Apply(context.Background(), fmt.Sprintf("token-%d", index))
			results <- result
			errorsFound <- applyErr
		}(index, planner)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsFound)
	for applyErr := range errorsFound {
		if applyErr != nil {
			t.Fatal(applyErr)
		}
	}
	reused := 0
	for result := range results {
		if result.Status != "completed" {
			t.Fatalf("result = %+v", result)
		}
		for _, step := range result.Steps {
			if step.Name == "setup" && step.Status == "reused" {
				reused++
			}
		}
	}
	setupCalls := countEvent(gits[0].events, "setup") + countEvent(gits[1].events, "setup")
	if setupCalls != 1 || reused != 1 || store.receiptCreates != 1 {
		t.Fatalf("setupCalls=%d reused=%d receiptCreates=%d events=%v/%v", setupCalls, reused, store.receiptCreates, gits[0].events, gits[1].events)
	}
}

func TestApplyWaitsForReceiptDurabilityRecoveryBeforePushAndJira(t *testing.T) {
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	git.snapshot.SetupHash = "setup-hash"
	injected := errors.New("injected receipt directory sync failure")
	committed := &state.SetupReceiptCommittedError{Key: strings.Repeat("a", 64), Ambiguous: true, Err: injected}
	store := &fakePlanStore{receiptCreateErr: committed, receiptReuseErr: injected}
	j := &fakeJiraGateway{issue: jiraIssueInProgress()}
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig()}, fakeJiraProvider{gateway: j})
	planned, err := planner.build(context.Background(), Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"})
	if err != nil {
		t.Fatal(err)
	}
	store.claimed = planned.payload
	store.claimRecord = state.Record{Fingerprint: planned.fingerprint}
	for attempt := 0; attempt < 2; attempt++ {
		git.events, j.events = nil, nil
		result, applyErr := planner.Apply(context.Background(), fmt.Sprintf("token-%d", attempt))
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if result.Status != "partial" || slices.Contains(git.events, "push") || slices.Contains(j.events, "fields") || slices.Contains(j.events, "transition") {
			t.Fatalf("attempt=%d result=%+v git=%v jira=%v", attempt, result, git.events, j.events)
		}
		store.receiptCreateErr = nil
	}
	store.receiptReuseErr = nil
	git.events, j.events = nil, nil
	result, err := planner.Apply(context.Background(), "token-recovered")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || !slices.Contains(git.events, "push") || slices.Contains(git.events, "setup") {
		t.Fatalf("recovered result=%+v git=%v jira=%v", result, git.events, j.events)
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
	applied  int
	events   *[]string
	applyErr error
}

type mutableConfigGateway struct {
	config    config.Config
	migration configMigration
}

func (f *mutableConfigGateway) Load(config.Paths) (config.Config, config.Source, error) {
	return f.config, config.SourceCanonical, nil
}

func (f *mutableConfigGateway) InspectMigration(config.Paths) (configMigrationInspection, error) {
	if f.migration == nil {
		return nil, nil
	}
	return fakeMigrationInspection{configMigration: f.migration, config: f.config, fingerprint: "mutable-migration"}, nil
}

func (f *fakeMigration) Apply(validate func(config.Config) error) error {
	f.applied++
	if f.applyErr != nil {
		return f.applyErr
	}
	if f.events != nil {
		*f.events = append(*f.events, "migration")
	}
	return validate(testConfig())
}
