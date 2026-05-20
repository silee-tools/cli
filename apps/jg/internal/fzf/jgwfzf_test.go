package fzf

import (
	"strings"
	"testing"
)

func TestWorktreePreviewCmdIncludesBranchAndLog(t *testing.T) {
	cmd := worktreePreviewCmd("/home/me")
	if !strings.Contains(cmd, "symbolic-ref") && !strings.Contains(cmd, "branch") {
		t.Errorf("preview cmd missing branch lookup: %q", cmd)
	}
	if !strings.Contains(cmd, "log -1") {
		t.Errorf("preview cmd missing log -1: %q", cmd)
	}
}
