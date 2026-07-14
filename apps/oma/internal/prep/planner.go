package prep

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/gitops"
	"github.com/silee-tools/oma/internal/jira"
	"github.com/silee-tools/oma/internal/state"
)

type planStore interface {
	Create(any, string) (state.Record, error)
	Claim(string, any) (state.Record, error)
	Consume(string) error
	EnsureSetupReceipt(string, func() error) (bool, error)
}

type gitGateway interface {
	Normalize(context.Context, string) (string, string, error)
	Fetch(context.Context, string) error
	RemoteBranch(context.Context, string, string) (bool, string, error)
	Inspect(context.Context, gitops.InspectRequest) (gitops.Snapshot, error)
	CreateWorktree(context.Context, gitops.Operation) error
	PrepareSubmodules(context.Context, string, []gitops.SubmoduleOperation) error
	RunSetup(context.Context, string, []string) error
	Push(context.Context, string, string) error
}

type jiraGateway interface {
	FetchIssue(context.Context, string) (jira.Issue, []byte, error)
	Myself(context.Context) (jira.User, error)
	Transitions(context.Context, string) ([]jira.Transition, error)
	UpdateFields(context.Context, string, map[string]any) error
	ApplyTransition(context.Context, string, string) error
	WriteSnapshot(string, []byte) error
}

type configMigration interface {
	Apply(func(config.Config) error) error
}
type configMigrationInspection interface {
	configMigration
	Fingerprint() string
	Load() (config.Config, error)
}
type configGateway interface {
	Load(config.Paths) (config.Config, config.Source, error)
	InspectMigration(config.Paths) (configMigrationInspection, error)
}
type jiraProvider interface {
	Open(config.Config, config.Paths) (jiraGateway, error)
}

type plannedIssue struct {
	Key             string          `json:"key"`
	Summary         string          `json:"summary"`
	DescriptionText string          `json:"description_text,omitempty"`
	Status          jira.Status     `json:"status"`
	Assignee        *jira.User      `json:"assignee,omitempty"`
	ProductType     json.RawMessage `json:"product_type,omitempty"`
	StartDate       json.RawMessage `json:"start_date,omitempty"`
}

type plannedTransition struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ToID       string `json:"to_id"`
	ToName     string `json:"to_name"`
	ToCategory string `json:"to_category"`
}

type plannedTransitionDecision struct {
	SelectedID     string          `json:"selected_id,omitempty"`
	Complete       bool            `json:"complete"`
	RequiredInputs []RequiredInput `json:"required_inputs,omitempty"`
}

type planPayload struct {
	Input                          Input                      `json:"input"`
	RepoRoot                       string                     `json:"repo_root"`
	CommonDir                      string                     `json:"common_dir"`
	Base                           Base                       `json:"base"`
	Branch                         string                     `json:"branch"`
	WorktreePath                   string                     `json:"worktree_path"`
	Git                            gitops.Snapshot            `json:"git"`
	Issue                          *plannedIssue              `json:"issue,omitempty"`
	JiraConfig                     *config.Config             `json:"jira_config,omitempty"`
	JiraSnapshotPath               string                     `json:"jira_snapshot_path,omitempty"`
	RequiredInputs                 []RequiredInput            `json:"required_inputs,omitempty"`
	OriginAvailable                bool                       `json:"origin_available"`
	RemoteBranchSHA                string                     `json:"remote_branch_sha,omitempty"`
	JiraTransitions                []plannedTransition        `json:"jira_transitions,omitempty"`
	JiraTransitionDecision         *plannedTransitionDecision `json:"jira_transition_decision,omitempty"`
	JiraConfigMigrationFingerprint string                     `json:"jira_config_migration_fingerprint,omitempty"`
}

type plannedBuild struct {
	payload     planPayload
	fingerprint string
	result      Result
	migration   configMigrationInspection
}

type Planner struct {
	paths         config.Paths
	store         planStore
	git           gitGateway
	configs       configGateway
	jiraProvider  jiraProvider
	now           func() time.Time
	canonicalPath func(string) (string, error)
}

func NewPlanner(paths config.Paths, store planStore, runner gitops.Runner, httpClient *http.Client) *Planner {
	return &Planner{
		paths: paths, store: store,
		git:           productionGit{runner: runner},
		configs:       productionConfig{},
		jiraProvider:  productionJiraProvider{httpClient: httpClient},
		now:           time.Now,
		canonicalPath: filepath.EvalSymlinks,
	}
}

func (p *Planner) Plan(ctx context.Context, input Input) (Result, error) {
	built, err := p.build(ctx, input)
	if err != nil {
		return Result{}, err
	}
	if len(built.result.RequiredInputs) != 0 {
		return built.result, nil
	}
	record, err := p.store.Create(built.payload, built.fingerprint)
	if err != nil {
		var committed *state.CommittedError
		if !errors.As(err, &committed) {
			return Result{}, fmt.Errorf("persist plan: %w", err)
		}
	}
	built.result.PlanToken = record.Token
	built.result.ExpiresAt = record.ExpiresAt
	built.result.Steps = append(built.result.Steps, Step{Name: "plan-state", Status: "completed", Detail: "로컬 승인 계획을 저장했습니다"})
	built.result.NextAction = "계획을 확인한 뒤 --plan과 --yes로 적용하세요"
	return built.result, nil
}

func (p *Planner) build(ctx context.Context, input Input) (plannedBuild, error) {
	if p.now == nil {
		p.now = time.Now
	}
	if input.Kind == "" {
		return plannedBuild{}, errors.New("oma prep: 작업 입력이 필요합니다")
	}
	repoRoot, _, err := p.git.Normalize(ctx, input.Repo)
	if err != nil {
		return plannedBuild{}, err
	}
	if err := p.git.Fetch(ctx, repoRoot); err != nil {
		return plannedBuild{}, err
	}

	title := input.Description
	var issue *jira.Issue
	var jiraConfig *config.Config
	var snapshotPath string
	var required []RequiredInput
	var plannedTransitions []plannedTransition
	var transitionDecision *plannedTransitionDecision
	var migration configMigrationInspection
	if input.Kind == InputJira {
		migration, err = p.configs.InspectMigration(p.paths)
		if err != nil {
			return plannedBuild{}, fmt.Errorf("inspect Jira configuration migration: %w", err)
		}
		var cfg config.Config
		if migration != nil {
			cfg, err = migration.Load()
		} else {
			cfg, _, err = p.configs.Load(p.paths)
		}
		if err != nil {
			return plannedBuild{}, fmt.Errorf("load Jira configuration: %w", err)
		}
		gateway, openErr := p.jiraProvider.Open(cfg, p.paths)
		if openErr != nil {
			return plannedBuild{}, openErr
		}
		fetched, raw, fetchErr := gateway.FetchIssue(ctx, input.IssueKey)
		if fetchErr != nil {
			return plannedBuild{}, fmt.Errorf("fetch Jira issue: %w", fetchErr)
		}
		issue = &fetched
		title = fetched.Summary
		snapshotPath, err = jiraSnapshotPath(p.paths.CacheRoot, cfg.JiraBaseURL, fetched.Key)
		if err != nil {
			return plannedBuild{}, err
		}
		if err := gateway.WriteSnapshot(snapshotPath, raw); err != nil {
			return plannedBuild{}, fmt.Errorf("write Jira snapshot: %w", err)
		}
		required, plannedTransitions, transitionDecision, err = requiredJiraInputs(ctx, gateway, cfg, fetched, input)
		if err != nil {
			return plannedBuild{}, err
		}
		jiraConfig = &cfg
	}

	names, err := BuildNames(input.Kind, input.BranchType, input.IssueKey, title, p.now())
	if err != nil {
		return plannedBuild{}, err
	}
	originAvailable, remoteBranchSHA, err := p.git.RemoteBranch(ctx, repoRoot, names.Branch)
	if err != nil {
		return plannedBuild{}, err
	}
	worktree := input.Worktree
	switch worktree {
	case "", "new":
		worktree = filepath.Join(repoRoot, ".worktrees", names.Worktree)
	case "current":
		worktree = repoRoot
	default:
		if !filepath.IsAbs(worktree) {
			worktree, err = filepath.Abs(worktree)
			if err != nil {
				return plannedBuild{}, fmt.Errorf("normalize worktree path: %w", err)
			}
		}
	}
	inspectionWorktree := worktree
	if input.Worktree == "current" {
		inspectionWorktree = "current"
	}
	snapshot, err := p.git.Inspect(ctx, gitops.InspectRequest{
		Repo: repoRoot, Base: input.Base, Branch: names.Branch, Worktree: inspectionWorktree,
		Submodules: input.Submodules, SetupArgs: input.SetupArgs, NoPush: input.NoPush,
	})
	if err != nil {
		return plannedBuild{}, err
	}

	payload := planPayload{
		Input: input, RepoRoot: snapshot.RepoRoot, CommonDir: snapshot.CommonDir,
		Base: Base{Ref: snapshot.BaseRef, SHA: snapshot.BaseSHA}, Branch: names.Branch,
		WorktreePath: worktree, Git: snapshot, JiraConfig: jiraConfig,
		JiraSnapshotPath: snapshotPath, RequiredInputs: required,
		OriginAvailable: originAvailable, RemoteBranchSHA: remoteBranchSHA,
		JiraTransitions: plannedTransitions, JiraTransitionDecision: transitionDecision,
	}
	if migration != nil {
		payload.JiraConfigMigrationFingerprint = migration.Fingerprint()
	}
	if issue != nil {
		payload.Issue = &plannedIssue{
			Key: issue.Key, Summary: issue.Summary, DescriptionText: issue.DescriptionText,
			Status: issue.Status, Assignee: issue.Assignee,
		}
		if jiraConfig != nil {
			payload.Issue.ProductType = append(json.RawMessage(nil), issue.CustomFields[jiraConfig.ProductTypeField]...)
			payload.Issue.StartDate = append(json.RawMessage(nil), issue.CustomFields[jiraConfig.StartDateField]...)
		}
	}
	fingerprint, err := planFingerprint(payload)
	if err != nil {
		return plannedBuild{}, err
	}
	result := resultFromPayload(payload)
	result.Status = "planned"
	result.Steps = append(result.Steps, Step{Name: "git-fetch", Status: "completed", Detail: "origin 상태를 조회했습니다"})
	if payload.Issue != nil {
		result.Steps = append(result.Steps, Step{Name: "jira-snapshot", Status: "completed", Detail: payload.JiraSnapshotPath})
		if payload.JiraConfigMigrationFingerprint != "" {
			result.Steps = append(result.Steps, Step{Name: "config-migration", Status: "planned", Detail: "호환 설정을 XDG 정본으로 전환합니다"})
		}
	}
	if len(required) != 0 {
		result.NextAction = "필수 입력을 지정해 계획을 다시 만드세요"
	}
	return plannedBuild{payload: payload, fingerprint: fingerprint, result: result, migration: migration}, nil
}

func requiredJiraInputs(ctx context.Context, gateway jiraGateway, cfg config.Config, issue jira.Issue, input Input) ([]RequiredInput, []plannedTransition, *plannedTransitionDecision, error) {
	var required []RequiredInput
	if cfg.ProductTypeField != "" && fieldEmpty(issue.CustomFields[cfg.ProductTypeField]) {
		if input.ProductType == "" {
			keys := make([]string, 0, len(cfg.ProductTypeOptions))
			for key := range cfg.ProductTypeOptions {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			options := make([]InputOption, 0, len(keys))
			for _, key := range keys {
				options = append(options, InputOption{Value: key, Label: cfg.ProductTypeOptions[key]})
			}
			required = append(required, RequiredInput{Kind: "product_type", Message: "Product type을 선택하세요", Options: options})
		} else if _, ok := cfg.ProductTypeOptions[input.ProductType]; !ok {
			return nil, nil, nil, fmt.Errorf("oma prep: 알 수 없는 Product type 설정 키입니다: %q", input.ProductType)
		}
	}
	available, err := gateway.Transitions(ctx, issue.Key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetch Jira transitions: %w", err)
	}
	sort.Slice(available, func(left, right int) bool {
		if available[left].ID != available[right].ID {
			return available[left].ID < available[right].ID
		}
		if available[left].To.ID != available[right].To.ID {
			return available[left].To.ID < available[right].To.ID
		}
		return available[left].Name < available[right].Name
	})
	observed := make([]plannedTransition, 0, len(available))
	for _, transition := range available {
		observed = append(observed, plannedTransition{ID: transition.ID, Name: transition.Name, ToID: transition.To.ID, ToName: transition.To.Name, ToCategory: transition.To.CategoryKey})
	}
	decision, err := jira.SelectTransition(issue.Status, available, input.TransitionID)
	if err != nil {
		return nil, nil, nil, err
	}
	plannedDecision := &plannedTransitionDecision{Complete: decision.Complete}
	if decision.Transition != nil {
		plannedDecision.SelectedID = decision.Transition.ID
	}
	for _, item := range decision.RequiredInputs {
		options := make([]InputOption, 0, len(item.Options))
		for _, option := range item.Options {
			options = append(options, InputOption{Value: option.ID, Label: option.Status})
		}
		sort.Slice(options, func(left, right int) bool { return options[left].Value < options[right].Value })
		normalized := RequiredInput{Kind: item.Kind, Message: item.Message, Options: options}
		required = append(required, normalized)
		plannedDecision.RequiredInputs = append(plannedDecision.RequiredInputs, normalized)
	}
	return required, observed, plannedDecision, nil
}

func fieldEmpty(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func planFingerprint(payload planPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode canonical plan: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func jiraSnapshotPath(cacheRoot, baseURL, key string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("jira base URL has no host")
	}
	return filepath.Join(cacheRoot, "jira", strings.ToLower(parsed.Hostname()), key+".json"), nil
}

func resultFromPayload(payload planPayload) Result {
	result := Result{
		InputKind: payload.Input.Kind, Branch: payload.Branch, WorktreePath: payload.WorktreePath,
		Base: payload.Base, JiraSnapshotPath: payload.JiraSnapshotPath,
		RequiredInputs: append([]RequiredInput(nil), payload.RequiredInputs...),
		Steps:          []Step{}, Warnings: []string{},
	}
	if payload.Issue != nil {
		assignee := ""
		if payload.Issue.Assignee != nil {
			assignee = payload.Issue.Assignee.DisplayName
		}
		result.IssueKey = payload.Issue.Key
		result.Issue = &IssueContext{
			Key: payload.Issue.Key, Summary: payload.Issue.Summary,
			DescriptionText: payload.Issue.DescriptionText, Status: payload.Issue.Status.Name, Assignee: assignee,
		}
	}
	return result
}

type productionConfig struct{}

func (productionConfig) Load(paths config.Paths) (config.Config, config.Source, error) {
	return config.Load(paths)
}
func (productionConfig) PlanMigration(paths config.Paths) (configMigration, error) {
	migration, err := config.PlanMigration(paths)
	if migration == nil || err != nil {
		return nil, err
	}
	return migration, nil
}
func (productionConfig) InspectMigration(paths config.Paths) (configMigrationInspection, error) {
	migration, err := config.InspectMigration(paths)
	if migration == nil || err != nil {
		return nil, err
	}
	return migration, nil
}

type productionJiraProvider struct{ httpClient *http.Client }

func (p productionJiraProvider) Open(cfg config.Config, paths config.Paths) (jiraGateway, error) {
	if strings.TrimSpace(cfg.JiraBaseURL) == "" || strings.TrimSpace(cfg.ProductTypeField) == "" || strings.TrimSpace(cfg.StartDateField) == "" {
		return nil, errors.New("jira configuration is missing jira_base_url, product_type_field, or start_date_field")
	}
	if len(cfg.ProductTypeOptions) == 0 {
		return nil, errors.New("jira configuration has no product_type_options")
	}
	parsed, err := url.Parse(cfg.JiraBaseURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("jira base URL has no host")
	}
	credentials, err := jira.CredentialsFromNetrc(paths.Netrc, parsed.Hostname())
	if err != nil {
		return nil, err
	}
	client, err := jira.NewClient(cfg.JiraBaseURL, p.httpClient, credentials)
	if err != nil {
		return nil, err
	}
	return productionJira{Client: client}, nil
}

type productionJira struct{ *jira.Client }

func (productionJira) WriteSnapshot(path string, raw []byte) error {
	return jira.WriteSnapshot(path, raw)
}

type productionGit struct{ runner gitops.Runner }

func (g productionGit) Normalize(ctx context.Context, repo string) (string, string, error) {
	return gitops.NormalizeRepo(ctx, g.runner, repo)
}
func (g productionGit) Fetch(ctx context.Context, repo string) error {
	return gitops.FetchOrigin(ctx, g.runner, repo)
}
func (g productionGit) RemoteBranch(ctx context.Context, repo, branch string) (bool, string, error) {
	remotes, err := g.runner.Run(ctx, "git", "-C", repo, "remote")
	if err != nil {
		return false, "", fmt.Errorf("list Git remotes: %w", err)
	}
	hasOrigin := false
	for _, remote := range strings.Fields(string(remotes)) {
		if remote == "origin" {
			hasOrigin = true
			break
		}
	}
	if !hasOrigin {
		return false, "", nil
	}
	output, err := g.runner.Run(ctx, "git", "-C", repo, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return false, "", fmt.Errorf("inspect remote branch %q: %w", branch, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return true, "", nil
	}
	if len(fields) < 2 {
		return false, "", fmt.Errorf("inspect remote branch %q: malformed response", branch)
	}
	return true, fields[0], nil
}
func (g productionGit) Inspect(ctx context.Context, request gitops.InspectRequest) (gitops.Snapshot, error) {
	return gitops.Inspect(ctx, g.runner, request)
}
func (g productionGit) CreateWorktree(ctx context.Context, operation gitops.Operation) error {
	if filepath.Clean(operation.Path) == filepath.Clean(operation.Repo) {
		return g.checkoutCurrent(ctx, operation)
	}
	return gitops.CreateWorktree(ctx, g.runner, operation)
}

func (g productionGit) checkoutCurrent(ctx context.Context, operation gitops.Operation) error {
	if _, err := g.runner.Run(ctx, "git", "-C", operation.Repo, "check-ref-format", "--branch", operation.Branch); err != nil {
		return fmt.Errorf("invalid branch %q: %w", operation.Branch, err)
	}
	output, err := g.runner.Run(ctx, "git", "-C", operation.Repo, "for-each-ref", "--format=%(objectname)", "refs/heads/"+operation.Branch)
	if err != nil {
		return fmt.Errorf("inspect local branch %q: %w", operation.Branch, err)
	}
	branchSHA := strings.TrimSpace(string(output))
	if branchSHA != "" && branchSHA != operation.BaseSHA {
		return fmt.Errorf("branch %q points to %s, expected %s", operation.Branch, branchSHA, operation.BaseSHA)
	}
	args := []string{"-C", operation.Repo, "checkout"}
	if branchSHA == "" {
		args = append(args, "-b", operation.Branch, operation.BaseSHA)
	} else {
		args = append(args, operation.Branch)
	}
	if _, err := g.runner.Run(ctx, "git", args...); err != nil {
		return fmt.Errorf("switch current worktree to %q: %w", operation.Branch, err)
	}
	return nil
}
func (g productionGit) PrepareSubmodules(ctx context.Context, root string, operations []gitops.SubmoduleOperation) error {
	return gitops.PrepareSubmodules(ctx, g.runner, root, operations)
}
func (g productionGit) RunSetup(ctx context.Context, root string, args []string) error {
	return gitops.RunSetup(ctx, root, args)
}
func (g productionGit) Push(ctx context.Context, repo, branch string) error {
	return gitops.Push(ctx, g.runner, repo, branch)
}
