package prep

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/gitops"
	"github.com/silee-tools/oma/internal/jira"
	"github.com/silee-tools/oma/internal/state"
)

func TestPlanDescriptionAndEmptyNeverTouchJiraConfiguration(t *testing.T) {
	for _, input := range []Input{
		{Kind: InputDescription, Description: "한글 작업", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"},
		{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"},
	} {
		store := &fakePlanStore{}
		git := &fakeGitGateway{snapshot: testGitSnapshot()}
		planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})

		result, err := planner.Plan(context.Background(), input)
		if err != nil {
			t.Fatalf("Plan(%s) error = %v", input.Kind, err)
		}
		if result.Status != "planned" || result.PlanToken == "" {
			t.Fatalf("Plan(%s) result = %+v", input.Kind, result)
		}
		if result.Issue != nil || result.JiraSnapshotPath != "" {
			t.Fatalf("Plan(%s) leaked Jira fields: %+v", input.Kind, result)
		}
		if git.writes != 0 {
			t.Fatalf("Plan(%s) external Git writes = %d, want 0", input.Kind, git.writes)
		}
	}
}

func TestPlanJiraSnapshotsAndWithholdsTokenForRequiredInput(t *testing.T) {
	store := &fakePlanStore{}
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	j := &fakeJiraGateway{
		issue: jira.Issue{
			Key: "ABC-123", Summary: "결제 수정",
			Status:       jira.Status{ID: "1", Name: "할 일", CategoryKey: "new"},
			CustomFields: map[string]json.RawMessage{"custom_product": json.RawMessage("null")},
		},
		raw:         []byte(`{"key":"ABC-123"}`),
		transitions: []jira.Transition{{ID: "21", To: jira.Status{ID: "2", Name: "진행 중", CategoryKey: "indeterminate"}}},
	}
	cfg := fakeConfigGateway{config: testConfig()}
	planner := testPlanner(store, git, cfg, fakeJiraProvider{gateway: j})

	result, err := planner.Plan(context.Background(), Input{
		Kind: InputJira, IssueKey: "ABC-123", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "planned" || result.PlanToken != "" {
		t.Fatalf("result = %+v, want planned without token", result)
	}
	if len(result.RequiredInputs) != 1 || result.RequiredInputs[0].Kind != "product_type" {
		t.Fatalf("required inputs = %+v", result.RequiredInputs)
	}
	if store.creates != 0 {
		t.Fatalf("store creates = %d, want 0", store.creates)
	}
	if len(j.snapshots) != 1 || result.JiraSnapshotPath == "" {
		t.Fatalf("snapshot calls = %v, result = %+v", j.snapshots, result)
	}
}

func TestPlanFingerprintChangesWithObservedState(t *testing.T) {
	store := &fakePlanStore{}
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	input := Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}

	if _, err := planner.Plan(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	first := store.fingerprints[0]
	git.snapshot.BaseSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := planner.Plan(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if first == store.fingerprints[1] {
		t.Fatal("fingerprint did not change with base SHA")
	}
}

func TestPlanCurrentWorktreeKeepsInspectionModeAndReturnsRepositoryPath(t *testing.T) {
	store := &fakePlanStore{}
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	planner := testPlanner(store, git, panicConfigGateway{}, panicJiraProvider{})
	result, err := planner.Plan(context.Background(), Input{Kind: InputEmpty, Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "current"})
	if err != nil {
		t.Fatal(err)
	}
	if len(git.inspectRequests) != 1 || git.inspectRequests[0].Worktree != "current" {
		t.Fatalf("inspect requests = %+v", git.inspectRequests)
	}
	if result.WorktreePath != "/repo" {
		t.Fatalf("worktree path = %q", result.WorktreePath)
	}
}

func TestPlanNormalizesJiraTransitionObservationOrder(t *testing.T) {
	store := &fakePlanStore{}
	git := &fakeGitGateway{snapshot: testGitSnapshot()}
	j := &fakeJiraGateway{
		issue: jira.Issue{Key: "ABC-123", Summary: "작업", Status: jira.Status{ID: "1", Name: "할 일", CategoryKey: "new"}, CustomFields: map[string]json.RawMessage{"custom_product": json.RawMessage(`{"value":"Feature"}`)}},
		transitions: []jira.Transition{
			{ID: "22", Name: "Start B", To: jira.Status{ID: "3", Name: "진행 B", CategoryKey: "indeterminate"}},
			{ID: "21", Name: "Start A", To: jira.Status{ID: "2", Name: "진행 A", CategoryKey: "indeterminate"}},
		},
	}
	planner := testPlanner(store, git, fakeConfigGateway{config: testConfig()}, fakeJiraProvider{gateway: j})
	input := Input{Kind: InputJira, IssueKey: "ABC-123", ProductType: "feature", Repo: "/repo", BranchType: "feature", Base: "main", Worktree: "new"}
	first, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	j.transitions[0], j.transitions[1] = j.transitions[1], j.transitions[0]
	second, err := planner.build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.fingerprint != second.fingerprint {
		t.Fatalf("transition order changed fingerprint: %s != %s", first.fingerprint, second.fingerprint)
	}
	if got := first.result.RequiredInputs[0].Options; got[0].Value != "21" || got[1].Value != "22" {
		t.Fatalf("required transition options = %+v", got)
	}
}

func testPlanner(store planStore, git gitGateway, configs configGateway, provider jiraProvider) *Planner {
	return &Planner{
		paths: config.Paths{CacheRoot: "/cache/oma", StateRoot: "/state/oma"},
		store: store, git: git, configs: configs, jiraProvider: provider,
		now:           func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.Local) },
		canonicalPath: func(path string) (string, error) { return filepath.Clean(path), nil },
	}
}

func testGitSnapshot() gitops.Snapshot {
	return gitops.Snapshot{
		RepoRoot: "/repo", CommonDir: "/repo/.git", BaseRef: "main",
		BaseSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}

func testConfig() config.Config {
	return config.Config{
		JiraBaseURL: "https://jira.example.test", ProductTypeField: "custom_product",
		StartDateField: "custom_start", ProductTypeOptions: map[string]string{"feature": "Feature"},
	}
}

type fakePlanStore struct {
	mu               sync.Mutex
	payloads         []planPayload
	fingerprints     []string
	creates          int
	claimed          planPayload
	claimRecord      state.Record
	claimErr         error
	consumes         int
	receipts         map[string]bool
	receiptCreates   int
	receiptChecks    int
	receiptCreateErr error
	receiptReuseErr  error
	consumeErr       error
}

func (f *fakePlanStore) Create(payload any, fingerprint string) (state.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	value := payload.(planPayload)
	f.payloads = append(f.payloads, value)
	f.fingerprints = append(f.fingerprints, fingerprint)
	return state.Record{Token: "token-abcdefghijklmnopqrstuvwxyz0123456789", Fingerprint: fingerprint, ExpiresAt: time.Date(2026, 7, 14, 12, 30, 0, 0, time.UTC)}, nil
}

func (f *fakePlanStore) Claim(_ string, payload any) (state.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value := payload.(*planPayload)
	*value = f.claimed
	return f.claimRecord, f.claimErr
}
func (f *fakePlanStore) Consume(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumes++
	return f.consumeErr
}
func (f *fakePlanStore) EnsureSetupReceipt(key string, setup func() error) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.receiptChecks++
	if f.receipts != nil && f.receipts[key] {
		if f.receiptReuseErr != nil {
			return false, f.receiptReuseErr
		}
		return true, nil
	}
	if err := setup(); err != nil {
		return false, &state.SetupCallbackError{Err: err}
	}
	f.receiptCreates++
	if f.receiptCreateErr != nil {
		var committed *state.SetupReceiptCommittedError
		if errors.As(f.receiptCreateErr, &committed) {
			if f.receipts == nil {
				f.receipts = make(map[string]bool)
			}
			f.receipts[key] = true
		}
		return false, f.receiptCreateErr
	}
	if f.receipts == nil {
		f.receipts = make(map[string]bool)
	}
	f.receipts[key] = true
	return false, nil
}

type fakeGitGateway struct {
	snapshot        gitops.Snapshot
	events          []string
	writes          int
	failAt          string
	beforeWrite     func()
	inspectRequests []gitops.InspectRequest
	noOrigin        bool
	remoteSHA       string
}

func (f *fakeGitGateway) Normalize(context.Context, string) (string, string, error) {
	return f.snapshot.RepoRoot, f.snapshot.CommonDir, nil
}
func (f *fakeGitGateway) Fetch(context.Context, string) error {
	f.events = append(f.events, "fetch")
	return nil
}
func (f *fakeGitGateway) RemoteBranch(context.Context, string, string) (bool, string, error) {
	return !f.noOrigin, f.remoteSHA, nil
}
func (f *fakeGitGateway) Inspect(_ context.Context, request gitops.InspectRequest) (gitops.Snapshot, error) {
	f.inspectRequests = append(f.inspectRequests, request)
	f.events = append(f.events, "inspect")
	return f.snapshot, nil
}
func (f *fakeGitGateway) CreateWorktree(context.Context, gitops.Operation) error {
	if f.beforeWrite != nil {
		f.beforeWrite()
	}
	f.events = append(f.events, "worktree")
	f.writes++
	return f.maybeFail("worktree")
}
func (f *fakeGitGateway) PrepareSubmodules(context.Context, string, []gitops.SubmoduleOperation) error {
	f.events = append(f.events, "submodules")
	f.writes++
	return f.maybeFail("submodules")
}
func (f *fakeGitGateway) RunSetup(context.Context, string, []string) error {
	f.events = append(f.events, "setup")
	f.writes++
	return f.maybeFail("setup")
}
func (f *fakeGitGateway) Push(context.Context, string, string) error {
	f.events = append(f.events, "push")
	f.writes++
	return f.maybeFail("push")
}
func (f *fakeGitGateway) maybeFail(step string) error {
	if f.failAt == step {
		return errors.New("injected " + step + " failure")
	}
	return nil
}

type panicConfigGateway struct{}

func (panicConfigGateway) Load(config.Paths) (config.Config, config.Source, error) {
	panic("config accessed")
}
func (panicConfigGateway) PlanMigration(config.Paths) (configMigration, error) {
	panic("migration accessed")
}

type fakeConfigGateway struct {
	config             config.Config
	migration          configMigration
	planMigrationCalls *int
}

func (f fakeConfigGateway) Load(config.Paths) (config.Config, config.Source, error) {
	return f.config, config.SourceCanonical, nil
}
func (f fakeConfigGateway) PlanMigration(config.Paths) (configMigration, error) {
	if f.planMigrationCalls != nil {
		*f.planMigrationCalls = *f.planMigrationCalls + 1
	}
	return f.migration, nil
}

type panicJiraProvider struct{}

func (panicJiraProvider) Open(config.Config, config.Paths) (jiraGateway, error) {
	panic("Jira accessed")
}

type fakeJiraProvider struct{ gateway jiraGateway }

func (f fakeJiraProvider) Open(config.Config, config.Paths) (jiraGateway, error) {
	return f.gateway, nil
}

type fakeJiraGateway struct {
	issue       jira.Issue
	raw         []byte
	transitions []jira.Transition
	events      []string
	snapshots   []string
	failAt      string
	beforeWrite func()
}

func (f *fakeJiraGateway) FetchIssue(context.Context, string) (jira.Issue, []byte, error) {
	f.events = append(f.events, "fetch-issue")
	if f.failAt == "fetch-issue" {
		return jira.Issue{}, nil, errors.New("injected fetch failure")
	}
	return f.issue, f.raw, nil
}
func (f *fakeJiraGateway) Myself(context.Context) (jira.User, error) {
	f.events = append(f.events, "myself")
	return jira.User{AccountID: "me", DisplayName: "Current User"}, nil
}
func (f *fakeJiraGateway) Transitions(context.Context, string) ([]jira.Transition, error) {
	f.events = append(f.events, "transitions")
	return f.transitions, nil
}
func (f *fakeJiraGateway) UpdateFields(context.Context, string, map[string]any) error {
	if f.beforeWrite != nil {
		f.beforeWrite()
	}
	f.events = append(f.events, "fields")
	return f.jiraFail("fields")
}
func (f *fakeJiraGateway) ApplyTransition(context.Context, string, string) error {
	if f.beforeWrite != nil {
		f.beforeWrite()
	}
	f.events = append(f.events, "transition")
	return f.jiraFail("transition")
}
func (f *fakeJiraGateway) WriteSnapshot(path string, _ []byte) error {
	f.events = append(f.events, "snapshot")
	f.snapshots = append(f.snapshots, filepath.Clean(path))
	return f.jiraFail("snapshot")
}
func (f *fakeJiraGateway) jiraFail(step string) error {
	if f.failAt == step {
		return errors.New("injected " + step + " failure")
	}
	return nil
}
