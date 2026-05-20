package main

import (
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
		{Path: "/repo", IsMain: true},
		{Path: "/repo-wt1", IsMain: false},
		{Path: "/repo-wt2", IsMain: false},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current != "/repo-wt1" {
		t.Errorf("current = %q, want /repo-wt1", current)
	}
	if len(candidates) != 2 || candidates[0] != "/repo" || candidates[1] != "/repo-wt2" {
		t.Errorf("candidates = %v, want [/repo /repo-wt2]", candidates)
	}
}

func TestSplitCurrentDoesNotFalseMatchPrefix(t *testing.T) {
	// /repo 는 /repo-wt1 의 prefix 처럼 보이지만 separator 가 없으므로 매칭 X
	wts := []worktree.Worktree{
		{Path: "/repo", IsMain: true},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current != "" {
		t.Errorf("current = %q, want empty (false prefix match)", current)
	}
	if len(candidates) != 1 || candidates[0] != "/repo" {
		t.Errorf("candidates = %v, want [/repo]", candidates)
	}
}
