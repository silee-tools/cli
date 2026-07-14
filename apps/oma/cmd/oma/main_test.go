package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionLine(t *testing.T) {
	if got := versionLine("oma", "1.2.3"); got != "oma v1.2.3 © 2026 silee-tools" {
		t.Fatalf("got %q", got)
	}
}

func TestRunShowsPrepHelp(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"prep", "--help"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Usage: oma prep") {
		t.Fatalf("stdout = %q, want prep usage", got)
	}
}
