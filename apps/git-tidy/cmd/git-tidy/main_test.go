package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/silee-tools/git-tidy/internal/classify"
)

func TestParseArgs(t *testing.T) {
	t.Setenv("GIT_TIDY_STALE_DAYS", "") // 환경 변수 격리
	cases := []struct {
		args []string
		want options
	}{
		{[]string{}, options{staleDays: 20}},
		{[]string{"--run"}, options{run: true, staleDays: 20}},
		{[]string{"--run", "--no-tui"}, options{run: true, noTUI: true, staleDays: 20}},
		{[]string{"--stale-days=7"}, options{staleDays: 7}},
		{[]string{"--no-fetch"}, options{noFetch: true, staleDays: 20}},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args)
		if err != nil {
			t.Errorf("parseArgs(%v) error: %v", c.args, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseArgs(%v) = %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestParseArgsRejectsUnknown(t *testing.T) {
	if _, err := parseArgs([]string{"--bogus"}); err == nil {
		t.Error("알 수 없는 플래그는 오류여야 함")
	}
}

func TestVersionLine(t *testing.T) {
	want := "git-tidy v1.2.3 © 2026 silee-tools\n"
	if got := versionLine("git-tidy", "1.2.3"); got != want {
		t.Errorf("versionLine = %q, want %q", got, want)
	}
}

func TestDefaultStaleDaysFromEnv(t *testing.T) {
	t.Setenv("GIT_TIDY_STALE_DAYS", "30")
	opts, err := parseArgs([]string{})
	if err != nil || opts.staleDays != 30 {
		t.Errorf("GIT_TIDY_STALE_DAYS 기본값 적용 실패: got %+v err=%v, want staleDays=30", opts, err)
	}
}

func TestPrintTargetsShowsSignalDescriptionsAndAbsorbedEvidence(t *testing.T) {
	c := classify.Classified{
		ToDelete: []classify.Result{
			{Name: "claude/example-absorbed-branch", Signal: classify.SignalAbsorbed, AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서"},
		},
	}
	var out bytes.Buffer
	printTargetsTo(&out, c)
	s := out.String()
	for _, want := range []string{
		"  [absorbed]\n    같은 Jira 티켓의 더 최신 base 커밋",
		"base: 9a640b52f [ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("출력에 %q 가 없음:\n%s", want, s)
		}
	}
}

func TestParseArgsBase(t *testing.T) {
	t.Setenv("GIT_TIDY_BASE", "develop")
	o, err := parseArgs(nil)
	if err != nil || o.base != "develop" {
		t.Fatalf("환경 변수가 기준 브랜치여야 한다: %q err=%v", o.base, err)
	}
	o, err = parseArgs([]string{"--base=release/x"})
	if err != nil || o.base != "release/x" {
		t.Fatalf("--base 가 환경 변수를 덮어야 한다: %q err=%v", o.base, err)
	}
}
