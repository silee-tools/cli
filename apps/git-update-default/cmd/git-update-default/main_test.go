package main

import "testing"

func TestVersionLine(t *testing.T) {
	got := versionLine("git-update-default", "1.2.3")
	want := "git-update-default v1.2.3 © 2026 silee-tools\n"
	if got != want {
		t.Fatalf("versionLine = %q, want %q", got, want)
	}
}
