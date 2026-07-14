package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silee-tools/oma/internal/prep"
)

func TestVersionLine(t *testing.T) {
	if got := versionLine("oma", "1.2.3"); got != "oma v1.2.3 © 2026 silee-tools" {
		t.Fatalf("got %q", got)
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{
			name: "jira with every option",
			args: []string{"ABC-123", "--repo", "/repo", "--type", "hotfix", "--base", "main", "--worktree", "current", "--submodule", "one", "--submodule", "two", "--setup-arg", "a", "--setup-arg", "b", "--product-type", "service", "--transition-id", "31", "--no-push", "--dry-run", "--plan", "token", "--yes", "--json"},
			want: options{Input: prep.Input{Kind: prep.InputJira, IssueKey: "ABC-123", Repo: "/repo", BranchType: "hotfix", Base: "main", Worktree: "current", Submodules: []string{"one", "two"}, SetupArgs: []string{"a", "b"}, ProductType: "service", TransitionID: "31", NoPush: true}, DryRun: true, PlanToken: "token", Yes: true, JSON: true},
		},
		{name: "description defaults", args: []string{"--description", "작업 설명"}, want: options{Input: prep.Input{Kind: prep.InputDescription, Description: "작업 설명", BranchType: "feature", Worktree: "new"}}},
		{name: "empty defaults", args: []string{"--empty"}, want: options{Input: prep.Input{Kind: prep.InputEmpty, BranchType: "feature", Worktree: "new"}}},
		{name: "no input is deferred to prompt", args: nil, want: options{Input: prep.Input{BranchType: "feature", Worktree: "new"}}},
		{name: "jira and description conflict", args: []string{"ABC-123", "--description", "work"}, wantErr: true},
		{name: "jira and empty conflict", args: []string{"ABC-123", "--empty"}, wantErr: true},
		{name: "description and empty conflict", args: []string{"--description", "work", "--empty"}, wantErr: true},
		{name: "two jira keys", args: []string{"ABC-123", "DEF-456"}, wantErr: true},
		{name: "empty description", args: []string{"--description", ""}, wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOptions(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseOptions() error = nil, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseOptions() = %+v, want %+v", got, tc.want)
			}
		})
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

func TestInstallKeepsConsistentStateOnSignal(t *testing.T) {
	appRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	goModCacheOutput, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatalf("go env GOMODCACHE: %v", err)
	}
	goModCache := strings.TrimSpace(string(goModCacheOutput))

	for _, timing := range []struct {
		name      string
		phase     string
		committed bool
	}{
		{name: "before-commit", phase: "before", committed: false},
		{name: "after-commit", phase: "after", committed: true},
	} {
		t.Run(timing.name, func(t *testing.T) {
			for _, tc := range []struct {
				signal   string
				exitCode int
			}{
				{signal: "HUP", exitCode: 129},
				{signal: "INT", exitCode: 130},
				{signal: "TERM", exitCode: 143},
			} {
				t.Run(tc.signal, func(t *testing.T) {
					home := t.TempDir()
					fakeBin := filepath.Join(home, "fake-bin")
					prefix := filepath.Join(home, "homebrew")
					marker := filepath.Join(home, "signal-sent")
					target := filepath.Join(home, ".local", "bin", "oma")
					statePath := filepath.Join(prefix, "var", "silee-tools", "oma", "active-channel")

					if err := os.MkdirAll(fakeBin, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
						t.Fatal(err)
					}
					previousBinary := []byte("previous binary\n")
					if err := os.WriteFile(target, previousBinary, 0o755); err != nil {
						t.Fatal(err)
					}

					writeExecutable(t, filepath.Join(fakeBin, "brew"), `#!/bin/sh
set -eu
[ "$1" = "--prefix" ]
printf '%s\n' "$INSTALL_TEST_PREFIX"
`)
					writeExecutable(t, filepath.Join(fakeBin, "mv"), `#!/bin/sh
set -eu
last=""
for arg do
  last="$arg"
done
if [ "$last" = "$INSTALL_TEST_STATE_PATH" ] && [ ! -e "$INSTALL_TEST_MARKER" ]; then
  : >"$INSTALL_TEST_MARKER"
  if [ "$INSTALL_TEST_PHASE" = "before" ]; then
    kill -s "$INSTALL_TEST_SIGNAL" "$PPID"
    exit 75
  fi
  /bin/mv "$@"
  kill -s "$INSTALL_TEST_SIGNAL" "$PPID"
  exit 75
fi
/bin/mv "$@"
`)

					cmd := exec.Command("mise", "run", "install")
					cmd.Dir = appRoot
					cmd.Env = append(os.Environ(),
						"HOME="+home,
						"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
						"INSTALL_TEST_PREFIX="+prefix,
						"INSTALL_TEST_MARKER="+marker,
						"INSTALL_TEST_SIGNAL="+tc.signal,
						"INSTALL_TEST_STATE_PATH="+statePath,
						"INSTALL_TEST_PHASE="+timing.phase,
						"GOMODCACHE="+goModCache,
						"MISE_TRUSTED_CONFIG_PATHS="+appRoot,
						"MISE_YES=1",
					)
					output, runErr := cmd.CombinedOutput()
					gotExitCode := 0
					if runErr != nil {
						var exitErr *exec.ExitError
						if !errors.As(runErr, &exitErr) {
							t.Fatalf("mise run install error = %v\n%s", runErr, output)
						}
						gotExitCode = exitErr.ExitCode()
					}
					if gotExitCode != tc.exitCode {
						t.Errorf("mise run install exit code = %d, want %d\n%s", gotExitCode, tc.exitCode, output)
					}

					gotBinary, err := os.ReadFile(target)
					if err != nil {
						t.Fatalf("read restored binary: %v", err)
					}
					if timing.committed {
						if bytes.Equal(gotBinary, previousBinary) {
							t.Errorf("committed install restored the previous binary")
						}
						state, err := os.ReadFile(statePath)
						if err != nil {
							t.Fatalf("read committed runtime state: %v", err)
						}
						if got := string(state); got != "channel=dev\n" {
							t.Errorf("runtime state = %q, want channel=dev", got)
						}
					} else {
						if !bytes.Equal(gotBinary, previousBinary) {
							t.Errorf("installed binary was not restored: got %d bytes, want %d bytes", len(gotBinary), len(previousBinary))
						}
						if _, err := os.Stat(statePath); !os.IsNotExist(err) {
							t.Errorf("runtime state exists after interrupted install: %v", err)
						}
					}
				})
			}
		})
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
