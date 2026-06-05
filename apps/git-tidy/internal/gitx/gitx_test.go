package gitx

import (
	"reflect"
	"testing"
)

func TestParseBranchLines(t *testing.T) {
	// for-each-ref --format='%(refname:short)\x00%(upstream:track)\x00%(committerdate:unix)\x00%(subject)'
	input := "feature-a\x00[gone]\x001700000000\x00[ABC-1] gone branch\n" +
		"feature-b\x00\x001800000000\x00[ABC-2] no upstream\n" +
		"feature-c\x00[ahead 1]\x001900000000\x00[ABC-3] ahead branch\n"
	got := parseBranchLines(input)
	want := []BranchRef{
		{Name: "feature-a", UpstreamGone: true, HasUpstream: true, CommitUnix: 1700000000, Subject: "[ABC-1] gone branch"},
		{Name: "feature-b", UpstreamGone: false, HasUpstream: false, CommitUnix: 1800000000, Subject: "[ABC-2] no upstream"},
		{Name: "feature-c", UpstreamGone: false, HasUpstream: true, CommitUnix: 1900000000, Subject: "[ABC-3] ahead branch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranchLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseCommitLines(t *testing.T) {
	input := "9a640b52f\x001780376000\x00[ABC-1375] feat: 새 worktree 셋업\n" +
		"4d4f1d52f\x001780635662\x00[ABC-1399] fix(ci): landing-lighthouse\n"
	got := parseCommitLines(input)
	want := []CommitRef{
		{ShortHash: "9a640b52f", CommitUnix: 1780376000, Subject: "[ABC-1375] feat: 새 worktree 셋업"},
		{ShortHash: "4d4f1d52f", CommitUnix: 1780635662, Subject: "[ABC-1399] fix(ci): landing-lighthouse"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseCommitLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseWorktreeBranches(t *testing.T) {
	input := "worktree /a\nbranch refs/heads/main\n\nworktree /b\nbranch refs/heads/feature-x\n\nworktree /c\ndetached\n"
	got := parseWorktreeBranches(input)
	want := map[string]string{"main": "/a", "feature-x": "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktreeBranches mismatch\n got=%+v\nwant=%+v", got, want)
	}
}
