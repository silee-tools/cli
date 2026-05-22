package gitx

import (
	"reflect"
	"testing"
)

func TestParseBranchLines(t *testing.T) {
	// for-each-ref --format='%(refname:short)\x00%(upstream:track)\x00%(committerdate:unix)'
	input := "feature-a\x00[gone]\x001700000000\n" +
		"feature-b\x00\x001800000000\n" +
		"feature-c\x00[ahead 1]\x001900000000\n"
	got := parseBranchLines(input)
	want := []BranchRef{
		{Name: "feature-a", UpstreamGone: true, HasUpstream: true, CommitUnix: 1700000000},
		{Name: "feature-b", UpstreamGone: false, HasUpstream: false, CommitUnix: 1800000000},
		{Name: "feature-c", UpstreamGone: false, HasUpstream: true, CommitUnix: 1900000000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranchLines mismatch\n got=%+v\nwant=%+v", got, want)
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
