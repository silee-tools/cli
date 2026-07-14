package prep

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/gitops"
	"github.com/silee-tools/oma/internal/jira"
	"github.com/silee-tools/oma/internal/state"
)

func (p *Planner) Apply(ctx context.Context, token string) (result Result, resultErr error) {
	var approved planPayload
	record, err := p.store.Claim(token, &approved)
	if err != nil {
		var committed *state.CommittedError
		if !errors.As(err, &committed) || committed.State != state.Claimed {
			return Result{}, fmt.Errorf("claim approved plan: %w", err)
		}
	}
	defer func() {
		if consumeErr := p.store.Consume(token); consumeErr != nil {
			var committed *state.CommittedError
			if errors.As(consumeErr, &committed) && committed.State == state.Consumed {
				if committed.Ambiguous {
					result.Warnings = append(result.Warnings, "승인 계획은 소비됐지만 영구 저장 확인이 불확실합니다")
				}
				return
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("consume approved plan: %w", consumeErr))
		}
	}()

	if approved.Input.Kind == InputJira {
		migration, migrationErr := p.configs.PlanMigration(p.paths)
		if migrationErr != nil {
			return Result{}, fmt.Errorf("plan Jira configuration migration: %w", migrationErr)
		}
		if migration != nil {
			if migrationErr := migration.Apply(func(cfg config.Config) error {
				gateway, openErr := p.jiraProvider.Open(cfg, p.paths)
				if openErr != nil {
					return openErr
				}
				_, authErr := gateway.Myself(ctx)
				return authErr
			}); migrationErr != nil {
				return Result{}, fmt.Errorf("migrate Jira configuration: %w", migrationErr)
			}
		}
	}

	current, err := p.build(ctx, approved.Input)
	if err != nil {
		return Result{}, fmt.Errorf("revalidate approved plan: %w", err)
	}
	if record.Fingerprint != current.fingerprint {
		if len(current.result.RequiredInputs) != 0 {
			return current.result, nil
		}
		fresh, createErr := p.store.Create(current.payload, current.fingerprint)
		if createErr != nil {
			var committed *state.CommittedError
			if !errors.As(createErr, &committed) {
				return Result{}, fmt.Errorf("persist refreshed plan: %w", createErr)
			}
		}
		current.result.PlanToken = fresh.Token
		current.result.ExpiresAt = fresh.ExpiresAt
		current.result.Steps = append(current.result.Steps, Step{Name: "plan-state", Status: "completed", Detail: "변경된 상태의 새 승인 계획을 저장했습니다"})
		current.result.NextAction = "상태가 바뀌었습니다. 새 계획을 확인하고 다시 승인하세요"
		return current.result, nil
	}

	result = resultFromPayload(current.payload)
	result.Status = "completed"
	mutated := false
	addStep := func(name, status, detail string) {
		result.Steps = append(result.Steps, Step{Name: name, Status: status, Detail: detail})
	}
	fail := func(name string, stepErr error) (Result, error) {
		addStep(name, "failed", safeStepError(stepErr))
		if mutated {
			result.Status = "partial"
		} else {
			result.Status = "failed"
		}
		result.NextAction = "완료된 단계는 보존됩니다. 같은 입력으로 새 계획을 만들어 다시 실행하세요"
		return result, nil
	}

	operation := gitops.Operation{Repo: current.payload.RepoRoot, Path: current.payload.WorktreePath, Branch: current.payload.Branch, BaseSHA: current.payload.Base.SHA}
	if err := p.git.CreateWorktree(ctx, operation); err != nil {
		return fail("worktree", err)
	}
	mutated = true
	addStep("worktree", "completed", current.payload.WorktreePath)

	submoduleOperations := make([]gitops.SubmoduleOperation, 0, len(current.payload.Git.Submodules))
	for _, submodule := range current.payload.Git.Submodules {
		submoduleOperations = append(submoduleOperations, gitops.SubmoduleOperation{
			Path: submodule.Path, URL: submodule.URL, Branch: current.payload.Branch,
			BaseRef: submodule.BaseRef, BaseSHA: submodule.BaseSHA,
		})
	}
	if err := p.git.PrepareSubmodules(ctx, current.payload.WorktreePath, submoduleOperations); err != nil {
		return fail("submodules", err)
	}
	addStep("submodules", "completed", fmt.Sprintf("%d selected", len(submoduleOperations)))
	if current.payload.Git.SetupHash == "" {
		addStep("setup", "skipped", "setup script is absent")
	} else {
		receiptKey, err := p.setupReceiptKey(current.payload)
		if err != nil {
			return fail("setup-receipt", err)
		}
		reused, err := p.store.EnsureSetupReceipt(receiptKey, func() error {
			return p.git.RunSetup(ctx, current.payload.WorktreePath, current.payload.Input.SetupArgs)
		})
		if err != nil {
			return fail("setup-receipt", err)
		}
		if reused {
			addStep("setup", "reused", "durable setup receipt")
		} else {
			addStep("setup", "completed", "setup complete")
		}
	}

	if current.payload.Input.NoPush || !current.payload.OriginAvailable {
		addStep("push", "skipped", "--no-push")
		if current.payload.Input.NoPush {
			result.Warnings = append(result.Warnings, "--no-push에 따라 원격 브랜치 생성을 생략했습니다")
		} else {
			result.Warnings = append(result.Warnings, "origin이 없어 원격 브랜치 생성을 생략했습니다")
		}
	} else {
		if err := p.git.Push(ctx, current.payload.WorktreePath, current.payload.Branch); err != nil {
			return fail("push", err)
		}
		addStep("push", "completed", current.payload.Branch)
		for _, submodule := range current.payload.Git.Submodules {
			repo := filepath.Join(current.payload.WorktreePath, filepath.FromSlash(submodule.Path))
			if err := p.git.Push(ctx, repo, current.payload.Branch); err != nil {
				return fail("submodule-push", err)
			}
			addStep("submodule-push", "completed", submodule.Path)
		}
	}

	if current.payload.Input.Kind == InputJira {
		gateway, err := p.jiraProvider.Open(*current.payload.JiraConfig, p.paths)
		if err != nil {
			return fail("jira-open", err)
		}
		if err := applyJira(ctx, gateway, current.payload, p.now()); err != nil {
			return fail("jira", err)
		}
		addStep("jira", "completed", current.payload.Issue.Key)
		finalIssue, raw, err := gateway.FetchIssue(ctx, current.payload.Issue.Key)
		if err != nil {
			return fail("jira-final-snapshot", err)
		}
		if err := gateway.WriteSnapshot(current.payload.JiraSnapshotPath, raw); err != nil {
			return fail("jira-final-snapshot", err)
		}
		addStep("jira-final-snapshot", "completed", current.payload.JiraSnapshotPath)
		result.Issue = issueContext(finalIssue)
	}
	result.NextAction = "생성된 worktree로 이동해 작업을 시작하세요"
	return result, nil
}

func (p *Planner) setupReceiptKey(payload planPayload) (string, error) {
	canonicalize := p.canonicalPath
	if canonicalize == nil {
		canonicalize = filepath.EvalSymlinks
	}
	commonDir, err := canonicalize(payload.CommonDir)
	if err != nil {
		return "", fmt.Errorf("canonicalize Git common directory for setup receipt: %w", err)
	}
	worktree, err := canonicalize(payload.WorktreePath)
	if err != nil {
		return "", fmt.Errorf("canonicalize worktree for setup receipt: %w", err)
	}
	submodules := append([]gitops.Submodule(nil), payload.Git.Submodules...)
	sort.Slice(submodules, func(i, j int) bool {
		left, right := submodules[i], submodules[j]
		return left.Path < right.Path || (left.Path == right.Path && (left.URL < right.URL || (left.URL == right.URL && (left.BaseRef < right.BaseRef || (left.BaseRef == right.BaseRef && left.BaseSHA < right.BaseSHA)))))
	})
	identity := struct {
		CommonDir  string             `json:"common_dir"`
		Worktree   string             `json:"worktree"`
		Branch     string             `json:"branch"`
		BaseSHA    string             `json:"base_sha"`
		SetupHash  string             `json:"setup_hash"`
		SetupArgs  []string           `json:"setup_args"`
		Submodules []gitops.Submodule `json:"submodules"`
	}{commonDir, worktree, payload.Branch, payload.Base.SHA, payload.Git.SetupHash, payload.Input.SetupArgs, submodules}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode setup receipt identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func applyJira(ctx context.Context, gateway jiraGateway, payload planPayload, today time.Time) error {
	issue, _, err := gateway.FetchIssue(ctx, payload.Issue.Key)
	if err != nil {
		return err
	}
	fields := make(map[string]any)
	if issue.Assignee == nil {
		user, err := gateway.Myself(ctx)
		if err != nil {
			return err
		}
		fields["assignee"] = map[string]string{"accountId": user.AccountID}
	}
	if name := payload.JiraConfig.StartDateField; name != "" && fieldEmpty(issue.CustomFields[name]) {
		fields[name] = today.Format("2006-01-02")
	}
	if name := payload.JiraConfig.ProductTypeField; name != "" && fieldEmpty(issue.CustomFields[name]) {
		label, ok := payload.JiraConfig.ProductTypeOptions[payload.Input.ProductType]
		if !ok {
			return fmt.Errorf("unknown Product type key %q", payload.Input.ProductType)
		}
		fields[name] = map[string]string{"value": label}
	}
	if len(fields) != 0 {
		if err := gateway.UpdateFields(ctx, issue.Key, fields); err != nil {
			return err
		}
	}

	requested := payload.Input.TransitionID
	for count := 0; count < 3; count++ {
		issue, _, err = gateway.FetchIssue(ctx, issue.Key)
		if err != nil {
			return err
		}
		if issue.Status.CategoryKey == "indeterminate" {
			return nil
		}
		issue.Status.TransitionCount = count
		available, err := gateway.Transitions(ctx, issue.Key)
		if err != nil {
			return err
		}
		decision, err := jira.SelectTransition(issue.Status, available, requested)
		if err != nil {
			return err
		}
		if len(decision.RequiredInputs) != 0 {
			return errors.New("jira transition requires a new approved plan")
		}
		if decision.Complete {
			return nil
		}
		if decision.Transition == nil {
			return errors.New("jira transition decision is empty")
		}
		if err := gateway.ApplyTransition(ctx, issue.Key, decision.Transition.ID); err != nil {
			return err
		}
		requested = ""
	}
	issue, _, err = gateway.FetchIssue(ctx, issue.Key)
	if err != nil {
		return err
	}
	if issue.Status.CategoryKey == "indeterminate" {
		return nil
	}
	return errors.New("jira transition limit of 3 steps reached")
}

func issueContext(issue jira.Issue) *IssueContext {
	assignee := ""
	if issue.Assignee != nil {
		assignee = issue.Assignee.DisplayName
	}
	return &IssueContext{Key: issue.Key, Summary: issue.Summary, DescriptionText: issue.DescriptionText, Status: issue.Status.Name, Assignee: assignee}
}

func safeStepError(err error) string {
	if err == nil {
		return ""
	}
	return "단계 실행에 실패했습니다. 완료된 단계와 다음 행동을 확인하세요"
}
