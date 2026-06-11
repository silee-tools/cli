package fzf

import (
	"strings"
	"testing"

	"github.com/silee-tools/jg/internal/worktree"
)

func TestWorktreeLabel(t *testing.T) {
	cases := []struct {
		name string
		wt   worktree.Worktree
		want string
	}{
		{
			name: "main 은 마커와 브랜치를 함께 보여준다",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true},
			want: "▸ acme-app  main",
		},
		{
			name: "브랜치 basename 이 이름과 같으면 이름만",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/ABC-101-login-timeout", Branch: "feature/ABC-101-login-timeout", IsMain: false},
			want: "  ABC-101-login-timeout",
		},
		{
			name: "브랜치 basename 이 이름과 다르면 브랜치 덧붙임",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/wt-foo", Branch: "feature/bar", IsMain: false},
			want: "  wt-foo  feature/bar",
		},
		{
			name: "detached 는 (detached) 덧붙임",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/hotfix", Branch: "", IsMain: false},
			want: "  hotfix  (detached)",
		},
		{
			name: "main 이면서 이름과 브랜치 basename 이 같으면 마커만",
			wt:   worktree.Worktree{Path: "/home/me/repos/main", Branch: "main", IsMain: true},
			want: "▸ main",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := worktreeLabel(c.wt); got != c.want {
				t.Errorf("worktreeLabel = %q, want %q", got, c.want)
			}
		})
	}
}

func TestWorktreePreviewCmdIncludesBranchAndLog(t *testing.T) {
	cmd := worktreePreviewCmd("/home/me")
	if !strings.Contains(cmd, "symbolic-ref") && !strings.Contains(cmd, "branch") {
		t.Errorf("preview cmd missing branch lookup: %q", cmd)
	}
	if !strings.Contains(cmd, "log -1") {
		t.Errorf("preview cmd missing log -1: %q", cmd)
	}
}

func TestBuildWorktreeInput(t *testing.T) {
	cur := worktree.Worktree{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true}
	in := WorktreeListPickerInput{
		Current: &cur,
		Candidates: []worktree.Worktree{
			{Path: "/home/me/repos/acme-app/.worktrees/ABC-101-login-timeout", Branch: "feature/ABC-101-login-timeout"},
			{Path: "/home/me/repos/acme-app/.worktrees/ABC-102-upload-retry", Branch: "feature/ABC-102-upload-retry"},
		},
	}
	input, headerLines := buildWorktreeInput(in)
	if headerLines != 1 {
		t.Fatalf("headerLines = %d, want 1 (Current 가 있으면 헤더 줄 1개)", headerLines)
	}
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (헤더 1 + 후보 2)\n%s", len(lines), input)
	}
	if lines[0] != "-1\t▸ acme-app  main" {
		t.Errorf("header line = %q, want %q", lines[0], "-1\t▸ acme-app  main")
	}
	if lines[1] != "0\t  ABC-101-login-timeout" {
		t.Errorf("candidate[0] = %q", lines[1])
	}
	if lines[2] != "1\t  ABC-102-upload-retry" {
		t.Errorf("candidate[1] = %q", lines[2])
	}
}

func TestBuildWorktreeInputNoCurrent(t *testing.T) {
	in := WorktreeListPickerInput{
		Candidates: []worktree.Worktree{
			{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true},
		},
	}
	input, headerLines := buildWorktreeInput(in)
	if headerLines != 0 {
		t.Fatalf("headerLines = %d, want 0 (Current 없음)", headerLines)
	}
	if got := strings.TrimRight(input, "\n"); got != "0\t▸ acme-app  main" {
		t.Errorf("input = %q", got)
	}
}

func TestSelectedWorktreeIndex(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOk bool
	}{
		{"0\t  ABC-101-login-timeout\n", 0, true},
		{"2\t▸ acme-app  main", 2, true},
		{"", 0, false},
		{"notanumber\tlabel", 0, false},
	}
	for _, c := range cases {
		got, ok := selectedWorktreeIndex(c.in)
		if got != c.want || ok != c.wantOk {
			t.Errorf("selectedWorktreeIndex(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.wantOk)
		}
	}
}
