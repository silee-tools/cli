package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silee-tools/oma/internal/prep"
)

type fakeWorkflow struct {
	plans       []prep.Result
	applyResult prep.Result
	inputs      []prep.Input
	applyTokens []string
}

func (f *fakeWorkflow) Plan(_ context.Context, input prep.Input) (prep.Result, error) {
	f.inputs = append(f.inputs, input)
	if len(f.plans) == 0 {
		return prep.Result{}, errors.New("unexpected Plan call")
	}
	result := f.plans[0]
	f.plans = f.plans[1:]
	return result, nil
}

func (f *fakeWorkflow) Apply(_ context.Context, token string) (prep.Result, error) {
	f.applyTokens = append(f.applyTokens, token)
	return f.applyResult, nil
}

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
			args: []string{"ABC-123", "--repo", "/repo", "--type", "hotfix", "--base", "main", "--worktree", "current", "--submodule", "one", "--submodule", "two", "--setup-arg", "a", "--setup-arg", "b", "--product-type", "service", "--transition-id", "31", "--no-push", "--dry-run", "--yes", "--json"},
			want: options{Input: prep.Input{Kind: prep.InputJira, IssueKey: "ABC-123", Repo: "/repo", BranchType: "hotfix", Base: "main", Worktree: "current", Submodules: []string{"one", "two"}, SetupArgs: []string{"a", "b"}, ProductType: "service", TransitionID: "31", NoPush: true}, DryRun: true, Yes: true, JSON: true},
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

func TestRunHiddenProductTypeCompletionUsesTOMLParserAndNoWorkflow(t *testing.T) {
	configHome := t.TempDir()
	configPath := filepath.Join(configHome, "oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	contents := `jira_base_url = "https://jira.example.com"
product_type_options = { "quoted key" = "Quoted", feature = "Feature", "한글" = "Korean" }
`
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	workflow := &fakeWorkflow{}
	var stdout bytes.Buffer
	if err := run([]string{"__complete", "product-types"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{Workflow: workflow}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "feature\x00quoted key\x00한글\x00"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if len(workflow.inputs) != 0 || len(workflow.applyTokens) != 0 {
		t.Fatalf("hidden completion touched workflow: %+v", workflow)
	}
}

func TestRunHiddenProductTypeCompletionRejectsInvalidTOML(t *testing.T) {
	configHome := t.TempDir()
	configPath := filepath.Join(configHome, "oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("product_type_options = { invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	var stdout bytes.Buffer
	err := run([]string{"__complete", "product-types"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{})
	if err == nil || !strings.Contains(fmt.Sprint(err), "configuration") || stdout.Len() != 0 {
		t.Fatalf("error = %v, stdout = %q", err, stdout.String())
	}
}

func TestRunHiddenProductTypeCompletionRejectsEmptyProtocolKey(t *testing.T) {
	configHome := t.TempDir()
	configPath := filepath.Join(configHome, "oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("product_type_options = { \"\" = \"Empty\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	var stdout bytes.Buffer
	err := run([]string{"__complete", "product-types"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{})
	if err == nil || !strings.Contains(err.Error(), "invalid completion key") || stdout.Len() != 0 {
		t.Fatalf("error = %v, stdout = %q", err, stdout.String())
	}
}

func TestRunHiddenProductTypeCompletionMissingConfigIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout bytes.Buffer
	if err := run([]string{"__complete", "product-types"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{}); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunHiddenProductTypeCompletionUsesHomeLegacyFallback(t *testing.T) {
	home := t.TempDir()
	legacyPath := filepath.Join(home, ".config", "prep-task", "config.toml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("product_type_options = { maintenance = \"Maintenance\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	var stdout bytes.Buffer
	if err := run([]string{"__complete", "product-types"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "maintenance\x00"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestHiddenCompletionBypassesRuntimeChannelLookup(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "oma")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build oma: %v\n%s", err, output)
	}
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "brew-called")
	writeExecutable(t, filepath.Join(fakeBin, "brew"), "#!/bin/sh\n: >\"$BREW_CALLED\"\nexit 97\n")
	configPath := filepath.Join(root, "config", "oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("product_type_options = { feature = \"Feature\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "__complete", "product-types")
	cmd.Env = append(os.Environ(), "PATH="+fakeBin, "BREW_CALLED="+marker, "XDG_CONFIG_HOME="+filepath.Dir(filepath.Dir(configPath)))
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "feature\x00" {
		t.Fatalf("stdout = %q", output)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime channel lookup called brew: %v", err)
	}
}

func TestRunInteractiveCollectsRequiredInputsThenApprovesExactPlan(t *testing.T) {
	workflow := &fakeWorkflow{
		plans: []prep.Result{
			{Status: "planned", RequiredInputs: []prep.RequiredInput{{
				Kind: "product_type", Message: "Product type",
				Options: []prep.InputOption{{Value: "feature", Label: "Feature"}},
			}}},
			{Status: "planned", PlanToken: "approved-token", Branch: "feature/work"},
		},
		applyResult: prep.Result{Status: "completed", Branch: "feature/work", WorktreePath: "/repo/.worktrees/work"},
	}
	prompt := &fakePrompter{selections: []string{"feature"}, confirmed: true}
	var stdout, stderr bytes.Buffer
	err := run([]string{"prep", "ABC-123", "--base", "main"}, strings.NewReader(""), &stdout, &stderr, dependencies{
		IsTerminal: func() bool { return true }, Prompter: prompt, Workflow: workflow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(workflow.inputs) != 2 || workflow.inputs[1].ProductType != "feature" {
		t.Fatalf("plan inputs = %+v", workflow.inputs)
	}
	if !reflect.DeepEqual(workflow.applyTokens, []string{"approved-token"}) {
		t.Fatalf("apply tokens = %v", workflow.applyTokens)
	}
	if len(prompt.confirms) != 1 || !strings.Contains(stdout.String(), "completed") {
		t.Fatalf("confirms = %v stdout = %q", prompt.confirms, stdout.String())
	}
}

func TestRunJSONKeepsStdoutAsOneDocumentAndDoesNotApprove(t *testing.T) {
	workflow := &fakeWorkflow{plans: []prep.Result{{Status: "planned", PlanToken: "opaque", InputKind: prep.InputEmpty}}}
	var stdout, stderr bytes.Buffer
	err := run([]string{"prep", "--empty", "--base", "main", "--dry-run", "--json"}, strings.NewReader(""), &stdout, &stderr, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("stdout = %q: %v", stdout.String(), err)
	}
	if len(workflow.applyTokens) != 0 {
		t.Fatalf("--json approved apply: %v", workflow.applyTokens)
	}
	if stdout.String() == "" || stderr.String() == "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMapsPartialResultToNonzeroAfterRendering(t *testing.T) {
	workflow := &fakeWorkflow{applyResult: prep.Result{Status: "partial", InputKind: prep.InputEmpty, NextAction: "retry"}}
	var stdout bytes.Buffer
	err := run([]string{"prep", "--plan", "opaque", "--yes", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("error = %v", err)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout is not JSON: %q", stdout.String())
	}
}

func TestRunNonInteractiveRequiredInputRendersAndFails(t *testing.T) {
	workflow := &fakeWorkflow{plans: []prep.Result{{
		Status: "planned", InputKind: prep.InputJira,
		RequiredInputs: []prep.RequiredInput{{Kind: "transition_id", Message: "전환 선택"}},
	}}}
	var stdout bytes.Buffer
	err := run([]string{"prep", "ABC-123", "--base", "main", "--dry-run", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err == nil || !strings.Contains(err.Error(), "필수 입력") {
		t.Fatalf("error = %v", err)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout is not JSON: %q", stdout.String())
	}
}

func TestRunRejectsDryRunWithPlanWithoutApplying(t *testing.T) {
	workflow := &fakeWorkflow{}
	err := run([]string{"prep", "--plan", "opaque", "--yes", "--dry-run", "--json"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err == nil || !strings.Contains(err.Error(), "--dry-run") {
		t.Fatalf("error = %v", err)
	}
	if len(workflow.applyTokens) != 0 {
		t.Fatalf("Apply calls = %v", workflow.applyTokens)
	}
}

func TestRunPlanTokenOnlyAppliesWithoutInputOrBase(t *testing.T) {
	workflow := &fakeWorkflow{applyResult: prep.Result{Status: "completed", InputKind: prep.InputEmpty}}
	var stdout bytes.Buffer
	err := run([]string{"prep", "--plan", "opaque", "--yes", "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(workflow.applyTokens, []string{"opaque"}) {
		t.Fatalf("Apply calls = %v", workflow.applyTokens)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsPlanTokenMixedWithNewPlanningInput(t *testing.T) {
	for _, args := range [][]string{
		{"prep", "ABC-123", "--plan", "opaque", "--yes"},
		{"prep", "--plan", "opaque", "--yes", "--base", "main"},
		{"prep", "--plan", "opaque", "--yes", "--description", "new work"},
	} {
		workflow := &fakeWorkflow{}
		err := run(args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{IsTerminal: func() bool { return false }, Workflow: workflow})
		if err == nil || !strings.Contains(err.Error(), "--plan") {
			t.Fatalf("run(%v) error = %v", args, err)
		}
		if len(workflow.applyTokens) != 0 {
			t.Fatalf("run(%v) Apply calls = %v", args, workflow.applyTokens)
		}
	}
}

func TestRunNonInteractivePlanAlwaysRequiresYes(t *testing.T) {
	workflow := &fakeWorkflow{}
	err := run([]string{"prep", "--plan", "opaque", "--json"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		IsTerminal: func() bool { return false }, Workflow: workflow,
	})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
	if len(workflow.applyTokens) != 0 {
		t.Fatalf("Apply calls = %v", workflow.applyTokens)
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
