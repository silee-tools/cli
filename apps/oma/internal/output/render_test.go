package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/silee-tools/oma/internal/prep"
)

func TestJSONRendersOneDocumentAndOmitsNonJiraFields(t *testing.T) {
	result := prep.Result{Status: "planned", PlanToken: "opaque-token", ExpiresAt: time.Date(2026, 7, 14, 3, 30, 0, 0, time.UTC), InputKind: prep.InputDescription, Base: prep.Base{Ref: "main", SHA: "abc"}, Branch: "feature/work", WorktreePath: "/repo/.worktrees/work"}
	var output bytes.Buffer
	if err := JSON(&output, result); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("JSON renderer emitted more than one document")
	}
	if _, ok := document["issue"]; ok {
		t.Fatal("non-Jira result contains issue")
	}
	if _, ok := document["jira_snapshot_path"]; ok {
		t.Fatal("non-Jira result contains snapshot path")
	}
	for _, key := range []string{"status", "plan_token", "expires_at", "input_kind", "base", "branch", "worktree_path", "steps", "warnings", "required_inputs", "next_action"} {
		if _, ok := document[key]; !ok {
			t.Errorf("JSON missing %q", key)
		}
	}
	base := document["base"].(map[string]any)
	if base["ref"] != "main" || base["sha"] != "abc" {
		t.Fatalf("base = %+v, want lowercase ref and sha", base)
	}
}

func TestJSONIncludesJiraContextButNoCredentialMaterial(t *testing.T) {
	result := prep.Result{Status: "planned", InputKind: prep.InputJira, IssueKey: "ABC-123", Issue: &prep.IssueContext{Key: "ABC-123", Summary: "작업", DescriptionText: "설명", Status: "할 일", Assignee: "담당자"}, JiraSnapshotPath: "/cache/ABC-123.json"}
	var output bytes.Buffer
	if err := JSON(&output, result); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, required := range []string{`"issue"`, `"jira_snapshot_path"`, `"description_text"`} {
		if !strings.Contains(got, required) {
			t.Errorf("JSON %q missing %s", got, required)
		}
	}
	for _, forbidden := range []string{"password", "Authorization", "netrc"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("JSON contains %q", forbidden)
		}
	}
}

func TestHumanRendersStatusAndNextAction(t *testing.T) {
	var output bytes.Buffer
	if err := Human(&output, prep.Result{Status: "partial", Branch: "feature/work", NextAction: "같은 명령을 다시 실행하세요"}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "partial") || !strings.Contains(got, "같은 명령") {
		t.Fatalf("human output = %q", got)
	}
}
