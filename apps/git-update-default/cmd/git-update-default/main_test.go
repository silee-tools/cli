package main

import "testing"

func TestVersionLine(t *testing.T) {
	got := versionLine("git-update-default", "1.2.3")
	want := "git-update-default v1.2.3 © 2026 silee-tools\n"
	if got != want {
		t.Fatalf("versionLine = %q, want %q", got, want)
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args      []string
		wantStash bool
		wantForce bool
		wantErr   bool
	}{
		{nil, false, false, false},
		{[]string{"--stash"}, true, false, false},
		{[]string{"--force"}, false, true, false},
		{[]string{"--bogus"}, false, false, true},
	}
	for _, c := range cases {
		o, err := parseArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Fatalf("parseArgs(%v) err=%v wantErr=%v", c.args, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if o.stash != c.wantStash || o.force != c.wantForce {
			t.Fatalf("parseArgs(%v) = %+v", c.args, o)
		}
	}
}

func TestDirtyPath(t *testing.T) {
	cases := []struct {
		tty, stash, force bool
		want              dirtyAction
	}{
		{true, false, false, pathInteractive},
		{false, false, false, pathCancel},
		{false, true, false, pathStash},
		{false, false, true, pathForce},
		{true, true, false, pathStash}, // 플래그가 있으면 TTY 여도 묻지 않는다
		{true, false, true, pathForce},
		{true, true, true, pathForce}, // force 가 stash 보다 우선
	}
	for _, c := range cases {
		got := dirtyPath(c.tty, c.stash, c.force)
		if got != c.want {
			t.Fatalf("dirtyPath(tty=%v,stash=%v,force=%v) = %v want %v",
				c.tty, c.stash, c.force, got, c.want)
		}
	}
}
