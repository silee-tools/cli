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
