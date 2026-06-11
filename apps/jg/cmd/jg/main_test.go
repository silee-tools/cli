package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silee-tools/jg/internal/worktree"
)

func TestJgwFlowDecision(t *testing.T) {
	// cwd 가 repo 안이고 인자 없으면 flowA
	if got := decideFlow(true, nil); got != flowA {
		t.Errorf("decideFlow(true, nil) = %v, want flowA", got)
	}
	// cwd 가 repo 안이지만 pattern 명시되면 flowB
	if got := decideFlow(true, []string{"pat"}); got != flowB {
		t.Errorf("decideFlow(true, [pat]) = %v, want flowB", got)
	}
	// cwd 가 repo 밖이면 flowB
	if got := decideFlow(false, nil); got != flowB {
		t.Errorf("decideFlow(false, nil) = %v, want flowB", got)
	}
}

func TestToolNameFromArgv0(t *testing.T) {
	cases := []struct {
		argv0 string
		want  string
	}{
		{"/usr/local/bin/jg", "jg"},
		{"/opt/homebrew/bin/jgw", "jgw"},
		{"jg", "jg"},
		{"./jgw", "jgw"},
	}
	for _, c := range cases {
		got := toolName(c.argv0)
		if got != c.want {
			t.Errorf("toolName(%q) = %q, want %q", c.argv0, got, c.want)
		}
	}
}

func TestVersionLine(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"jg", "0.3.0", "jg v0.3.0 © 2026 silee-tools\n"},
		{"jg", "dev", "jg vdev © 2026 silee-tools\n"},
	}
	for _, tt := range tests {
		if got := versionLine(tt.name, tt.version); got != tt.want {
			t.Errorf("versionLine(%q, %q) = %q, want %q", tt.name, tt.version, got, tt.want)
		}
	}
}

func TestSplitMainFindsFirstMain(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo", Branch: "main", IsMain: true},
		{Path: "/repo-wt1", Branch: "feat", IsMain: false},
	}
	path, branch := splitMain(wts)
	if path != "/repo" || branch != "main" {
		t.Errorf("splitMain = (%q, %q), want (/repo, main)", path, branch)
	}
}

func TestSplitMainReturnsEmptyWhenNoMain(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo-wt1", Branch: "feat", IsMain: false},
	}
	path, branch := splitMain(wts)
	if path != "" || branch != "" {
		t.Errorf("splitMain = (%q, %q), want empty pair", path, branch)
	}
}

func TestSplitCurrentSeparatesCwd(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo", Branch: "main", IsMain: true},
		{Path: "/repo-wt1", Branch: "feature/x"},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current == nil || current.Path != "/repo-wt1" {
		t.Fatalf("current = %v, want /repo-wt1", current)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo" {
		t.Errorf("candidates = %v, want [/repo]", candidates)
	}
}

func TestSplitCurrentDoesNotFalseMatchPrefix(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo-wt1", Branch: "feature/x"},
		{Path: "/repo-wt10", Branch: "feature/y"},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current == nil || current.Path != "/repo-wt1" {
		t.Fatalf("current = %v, want /repo-wt1 (prefix /repo-wt10 과 혼동 금지)", current)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo-wt10" {
		t.Errorf("candidates = %v, want [/repo-wt10]", candidates)
	}
}

func TestSplitCurrentMatchesThroughSymlink(t *testing.T) {
	realDir := t.TempDir()
	wtReal := filepath.Join(realDir, "wt-real")
	wtRealSubdir := filepath.Join(wtReal, "subdir")
	if err := os.MkdirAll(wtRealSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(realDir, "wt-link")
	if err := os.Symlink(wtReal, link); err != nil {
		t.Fatal(err)
	}
	wts := []worktree.Worktree{
		{Path: "/repo", Branch: "main", IsMain: true},
		{Path: wtReal, Branch: "feature/x"},
	}
	cwd := filepath.Join(link, "subdir")
	candidates, current := splitCurrent(wts, cwd)
	if current == nil || current.Path != wtReal {
		t.Fatalf("current = %v, want %s", current, wtReal)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo" {
		t.Errorf("candidates = %v, want [/repo]", candidates)
	}
}

func TestStepHeaderOmitsCounterForSingleStage(t *testing.T) {
	tests := []struct {
		step, total int
		label, want string
	}{
		{1, 1, "worktree 선택", "[worktree 선택]"},
		{1, 2, "repo 선택", "[1/2 repo 선택]"},
		{2, 2, "worktree 선택", "[2/2 worktree 선택]"},
	}
	for _, tt := range tests {
		if got := stepHeader(tt.step, tt.total, tt.label); got != tt.want {
			t.Errorf("stepHeader(%d, %d, %q) = %q, want %q",
				tt.step, tt.total, tt.label, got, tt.want)
		}
	}
}
