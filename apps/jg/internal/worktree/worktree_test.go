package worktree

import (
	"reflect"
	"testing"
)

func TestParsePorcelain(t *testing.T) {
	input := `worktree /Users/x/repo
HEAD abc123
branch refs/heads/main

worktree /Users/x/repo-feature
HEAD def456
branch refs/heads/feature/foo

worktree /Users/x/repo-detached
HEAD ghi789
detached
`
	got, err := parsePorcelain([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	want := []Worktree{
		{Path: "/Users/x/repo", Branch: "main", IsMain: true},
		{Path: "/Users/x/repo-feature", Branch: "feature/foo", IsMain: false},
		{Path: "/Users/x/repo-detached", Branch: "", IsMain: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePorcelain mismatch\n got = %+v\nwant = %+v", got, want)
	}
}
