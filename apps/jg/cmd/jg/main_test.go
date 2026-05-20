package main

import "testing"

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
