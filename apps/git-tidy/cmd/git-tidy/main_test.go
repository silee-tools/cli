package main

import "testing"

func TestParseArgs(t *testing.T) {
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
