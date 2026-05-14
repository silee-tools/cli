package scheduler

import (
	"strings"
	"testing"
)

func TestBuildPlist(t *testing.T) {
	plist := BuildPlist(Config{
		JGPath:  "/usr/local/bin/jg",
		OutPath: "/tmp/jg/clean.log",
		ErrPath: "/tmp/jg/clean.err.log",
		Hour:    9,
		Minute:  0,
	})

	want := []string{
		"<string>com.silee-tools.jg.clean</string>",
		"<string>/usr/local/bin/jg</string>",
		"<string>clean</string>",
		"<key>Hour</key>",
		"<integer>9</integer>",
		"<key>Minute</key>",
		"<integer>0</integer>",
		"<key>StandardOutPath</key>",
		"<string>/tmp/jg/clean.log</string>",
		"<key>StandardErrorPath</key>",
		"<string>/tmp/jg/clean.err.log</string>",
	}
	for _, part := range want {
		if !strings.Contains(plist, part) {
			t.Fatalf("plist missing %q:\n%s", part, plist)
		}
	}
}

func TestBuildPlistEscapesXML(t *testing.T) {
	plist := BuildPlist(Config{
		JGPath:  "/tmp/a&b/jg",
		OutPath: "/tmp/jg/out.log",
		ErrPath: "/tmp/jg/err.log",
		Hour:    9,
		Minute:  0,
	})

	if !strings.Contains(plist, "/tmp/a&amp;b/jg") {
		t.Fatalf("expected escaped path:\n%s", plist)
	}
}

func TestEnsureSupportedRejectsNonDarwin(t *testing.T) {
	err := ensureSupported("linux")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "macOS launchd") {
		t.Fatalf("unexpected error: %v", err)
	}
}
