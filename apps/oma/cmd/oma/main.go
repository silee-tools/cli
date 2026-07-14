package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/silee-tools/oma/internal/config"
	"github.com/silee-tools/oma/internal/gitops"
	"github.com/silee-tools/oma/internal/output"
	"github.com/silee-tools/oma/internal/prep"
	"github.com/silee-tools/oma/internal/runtimechannel"
	"github.com/silee-tools/oma/internal/state"
	"golang.org/x/term"
)

var version = "dev"
var runtimeStatePath string

type candidateProvider interface {
	InputKinds() []promptOption
	Bases() []promptOption
}

type dependencies struct {
	IsTerminal    func() bool
	Prompter      Prompter
	Candidates    candidateProvider
	CandidatesFor func(string) candidateProvider
	Workflow      prepWorkflow
}

type prepWorkflow interface {
	Plan(context.Context, prep.Input) (prep.Result, error)
	Apply(context.Context, string) (prep.Result, error)
}

type options struct {
	Input     prep.Input
	PlanToken string
	DryRun    bool
	Yes       bool
	JSON      bool
	planMixed bool
}

func parseOptions(args []string) (options, error) {
	parsed := options{Input: prep.Input{BranchType: "feature", Worktree: "new"}}
	var description string
	var empty bool
	var submodules stringList
	var setupArgs stringList

	flags := flag.NewFlagSet("oma prep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&description, "description", "", "")
	flags.BoolVar(&empty, "empty", false, "")
	flags.StringVar(&parsed.Input.Repo, "repo", "", "")
	flags.StringVar(&parsed.Input.BranchType, "type", "feature", "")
	flags.StringVar(&parsed.Input.Base, "base", "", "")
	flags.StringVar(&parsed.Input.Worktree, "worktree", "new", "")
	flags.Var(&submodules, "submodule", "")
	flags.Var(&setupArgs, "setup-arg", "")
	flags.StringVar(&parsed.Input.ProductType, "product-type", "", "")
	flags.StringVar(&parsed.Input.TransitionID, "transition-id", "", "")
	flags.BoolVar(&parsed.Input.NoPush, "no-push", false, "")
	flags.BoolVar(&parsed.DryRun, "dry-run", false, "")
	flags.StringVar(&parsed.PlanToken, "plan", "", "")
	flags.BoolVar(&parsed.Yes, "yes", false, "")
	flags.BoolVar(&parsed.JSON, "json", false, "")

	if err := flags.Parse(interspersedArgs(args)); err != nil {
		return options{}, fmt.Errorf("oma prep: 옵션 해석 실패: %w", err)
	}
	positionals := flags.Args()
	if len(positionals) > 1 {
		return options{}, fmt.Errorf("oma prep: Jira 키는 하나만 지정할 수 있습니다")
	}
	visited := make(map[string]bool)
	flags.Visit(func(current *flag.Flag) { visited[current.Name] = true })
	if parsed.PlanToken != "" {
		if parsed.DryRun {
			return options{}, fmt.Errorf("oma prep: --dry-run과 --plan은 함께 사용할 수 없습니다")
		}
		planningFlags := []string{"description", "empty", "repo", "type", "base", "worktree", "submodule", "setup-arg", "product-type", "transition-id", "no-push"}
		mixed := len(positionals) != 0
		for _, name := range planningFlags {
			mixed = mixed || visited[name]
		}
		parsed.planMixed = mixed
	}

	descriptionSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "description" {
			descriptionSet = true
		}
	})
	if descriptionSet && strings.TrimSpace(description) == "" {
		return options{}, fmt.Errorf("oma prep: --description은 비어 있을 수 없습니다")
	}
	inputCount := len(positionals)
	if descriptionSet {
		inputCount++
	}
	if empty {
		inputCount++
	}
	if inputCount > 1 {
		return options{}, fmt.Errorf("oma prep: Jira 키, --description, --empty는 함께 사용할 수 없습니다")
	}

	switch {
	case len(positionals) == 1:
		key, err := prep.NormalizeIssueKey(positionals[0])
		if err != nil {
			return options{}, err
		}
		parsed.Input.Kind = prep.InputJira
		parsed.Input.IssueKey = key
	case descriptionSet:
		parsed.Input.Kind = prep.InputDescription
		parsed.Input.Description = description
	case empty:
		parsed.Input.Kind = prep.InputEmpty
	}
	parsed.Input.Submodules = append([]string(nil), submodules...)
	parsed.Input.SetupArgs = append([]string(nil), setupArgs...)
	return parsed, nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func interspersedArgs(args []string) []string {
	valueFlags := map[string]bool{
		"--description":   true,
		"--repo":          true,
		"--type":          true,
		"--base":          true,
		"--worktree":      true,
		"--submodule":     true,
		"--setup-arg":     true,
		"--product-type":  true,
		"--transition-id": true,
		"--plan":          true,
	}
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.SplitN(arg, "=", 2)[0]
			if valueFlags[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools", name, version)
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, deps dependencies) error {
	if len(args) == 2 && args[0] == "__complete" && args[1] == "product-types" {
		return writeProductTypeCompletions(stdout)
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, err := fmt.Fprint(stdout, rootHelp)
		return err
	}
	if len(args) == 2 && args[0] == "prep" && (args[1] == "-h" || args[1] == "--help") {
		_, err := fmt.Fprint(stdout, prepHelp)
		return err
	}
	if len(args) > 0 && args[0] == "prep" {
		parsed, err := parseOptions(args[1:])
		if err != nil {
			return err
		}
		if deps.Candidates == nil && deps.CandidatesFor != nil {
			deps.Candidates = deps.CandidatesFor(parsed.Input.Repo)
		}
		if err := completeOptions(&parsed, deps); err != nil {
			return err
		}
		if deps.Workflow == nil {
			return nil
		}
		return executePrep(context.Background(), parsed, stdout, stderr, deps)
	}
	return fmt.Errorf("oma: 지원하지 않는 인자입니다")
}

func writeProductTypeCompletions(stdout io.Writer) error {
	paths := config.ResolvePaths(os.Getenv, "")
	cfg, _, err := config.Load(paths)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("oma __complete product-types: load configuration: %w", err)
	}
	keys := make([]string, 0, len(cfg.ProductTypeOptions))
	for key := range cfg.ProductTypeOptions {
		if key == "" || strings.ContainsRune(key, '\x00') {
			return fmt.Errorf("oma __complete product-types: configuration contains an invalid completion key")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := io.WriteString(stdout, key+"\x00"); err != nil {
			return fmt.Errorf("oma __complete product-types: write candidate: %w", err)
		}
	}
	return nil
}

type gitCandidates struct {
	repo   string
	runner gitops.Runner
	err    *error
}

func (gitCandidates) InputKinds() []promptOption {
	return []promptOption{
		{Value: string(prep.InputJira), Label: "Jira 작업"},
		{Value: string(prep.InputDescription), Label: "작업 설명"},
		{Value: string(prep.InputEmpty), Label: "빈 작업"},
	}
}

func (g gitCandidates) Bases() []promptOption {
	root, _, err := gitops.NormalizeRepo(context.Background(), g.runner, g.repo)
	if err != nil {
		if g.err != nil {
			*g.err = err
		}
		return nil
	}
	defaultRef, candidates, err := gitops.DefaultBase(context.Background(), g.runner, root)
	if err != nil {
		if g.err != nil {
			*g.err = err
		}
		return nil
	}
	result := make([]promptOption, 0, len(candidates))
	for _, candidate := range candidates {
		label := candidate
		if candidate == defaultRef {
			label += " (기본값)"
		}
		result = append(result, promptOption{Value: candidate, Label: label})
	}
	return result
}

func (g gitCandidates) candidateError() error {
	if g.err == nil {
		return nil
	}
	return *g.err
}

func completeOptions(parsed *options, deps dependencies) error {
	interactive := deps.IsTerminal != nil && deps.IsTerminal()
	if !interactive {
		if parsed.PlanToken != "" {
			if !parsed.Yes {
				return fmt.Errorf("oma prep: 비대화형 적용에는 --yes가 필요합니다")
			}
			if parsed.planMixed {
				return fmt.Errorf("oma prep: --plan은 새 작업 입력이나 계획 옵션과 함께 사용할 수 없습니다")
			}
			return nil
		}
		if parsed.Input.Kind == "" {
			return fmt.Errorf("oma prep: 비대화형 실행에는 작업 입력이 필요합니다")
		}
		if parsed.Input.Base == "" {
			return fmt.Errorf("oma prep: 비대화형 실행에는 --base가 필요합니다")
		}
		if !parsed.DryRun && parsed.PlanToken == "" {
			return fmt.Errorf("oma prep: 비대화형 적용에는 --plan이 필요합니다")
		}
		if !parsed.DryRun && !parsed.Yes {
			return fmt.Errorf("oma prep: 비대화형 적용에는 --yes가 필요합니다")
		}
		return nil
	}
	if parsed.PlanToken != "" {
		if parsed.planMixed {
			return fmt.Errorf("oma prep: --plan은 새 작업 입력이나 계획 옵션과 함께 사용할 수 없습니다")
		}
		return nil
	}
	if deps.Prompter == nil {
		return fmt.Errorf("oma prep: 대화형 입력기를 사용할 수 없습니다")
	}
	if parsed.Input.Kind == "" {
		if deps.Candidates == nil {
			return fmt.Errorf("oma prep: 작업 입력 후보 공급자를 사용할 수 없습니다")
		}
		selected, err := deps.Prompter.Select("작업 입력을 선택하세요", deps.Candidates.InputKinds())
		if err != nil {
			return err
		}
		switch prep.InputKind(selected) {
		case prep.InputJira:
			value, err := deps.Prompter.Input("Jira 키를 입력하세요")
			if err != nil {
				return err
			}
			key, err := prep.NormalizeIssueKey(value)
			if err != nil {
				return err
			}
			parsed.Input.Kind = prep.InputJira
			parsed.Input.IssueKey = key
		case prep.InputDescription:
			value, err := deps.Prompter.Input("작업 설명을 입력하세요")
			if err != nil {
				return err
			}
			value = strings.TrimSpace(value)
			if value == "" {
				return fmt.Errorf("oma prep: 작업 설명은 비어 있을 수 없습니다")
			}
			parsed.Input.Kind = prep.InputDescription
			parsed.Input.Description = value
		case prep.InputEmpty:
			parsed.Input.Kind = prep.InputEmpty
		default:
			return fmt.Errorf("oma prep: 알 수 없는 작업 입력 종류입니다: %q", selected)
		}
	}
	if parsed.Input.Base == "" {
		if deps.Candidates == nil {
			return fmt.Errorf("oma prep: 기준 브랜치 후보 공급자를 사용할 수 없습니다")
		}
		bases := deps.Candidates.Bases()
		if provider, ok := deps.Candidates.(interface{ candidateError() error }); ok && provider.candidateError() != nil {
			return provider.candidateError()
		}
		selected, err := deps.Prompter.Select("기준 브랜치를 선택하세요", bases)
		if err != nil {
			return err
		}
		if selected == "" {
			return errCancelled
		}
		parsed.Input.Base = selected
	}
	if deps.Workflow == nil {
		approved, err := deps.Prompter.Confirm("이 계획을 적용할까요?")
		if err != nil {
			return err
		}
		if !approved {
			return errCancelled
		}
	}
	return nil
}

func executePrep(ctx context.Context, parsed options, stdout, stderr io.Writer, deps dependencies) error {
	interactive := deps.IsTerminal != nil && deps.IsTerminal()
	var result prep.Result
	var err error
	if parsed.PlanToken != "" {
		if interactive && !parsed.Yes {
			approved, confirmErr := deps.Prompter.Confirm("승인한 계획을 적용할까요?")
			if confirmErr != nil {
				return confirmErr
			}
			if !approved {
				return errCancelled
			}
		}
		_, _ = fmt.Fprintln(stderr, "승인한 계획을 적용합니다")
		result, err = deps.Workflow.Apply(ctx, parsed.PlanToken)
	} else {
		for {
			_, _ = fmt.Fprintln(stderr, "현재 상태로 계획을 만듭니다")
			result, err = deps.Workflow.Plan(ctx, parsed.Input)
			if err != nil {
				break
			}
			if len(result.RequiredInputs) == 0 || !interactive {
				break
			}
			for _, required := range result.RequiredInputs {
				options := make([]promptOption, 0, len(required.Options))
				for _, option := range required.Options {
					options = append(options, promptOption{Value: option.Value, Label: option.Label})
				}
				selected, selectErr := deps.Prompter.Select(required.Message, options)
				if selectErr != nil {
					return selectErr
				}
				switch required.Kind {
				case "product_type":
					parsed.Input.ProductType = selected
				case "transition_id":
					parsed.Input.TransitionID = selected
				default:
					return fmt.Errorf("oma prep: 지원하지 않는 필수 입력입니다: %s", required.Kind)
				}
			}
		}
		if err == nil && !parsed.DryRun && interactive && len(result.RequiredInputs) == 0 {
			if parsed.JSON {
				_ = output.Human(stderr, result)
			} else {
				_ = output.Human(stdout, result)
			}
			approved, confirmErr := deps.Prompter.Confirm("이 계획을 적용할까요?")
			if confirmErr != nil {
				return confirmErr
			}
			if !approved {
				return errCancelled
			}
			result, err = deps.Workflow.Apply(ctx, result.PlanToken)
		}
	}
	if err != nil {
		return fmt.Errorf("oma prep: %w", err)
	}
	if parsed.JSON {
		if err := output.JSON(stdout, result); err != nil {
			return fmt.Errorf("oma prep: JSON 출력 실패: %w", err)
		}
	} else if err := output.Human(stdout, result); err != nil {
		return fmt.Errorf("oma prep: 출력 실패: %w", err)
	}
	if !interactive && len(result.RequiredInputs) != 0 {
		return fmt.Errorf("oma prep: 비대화형 실행에 필수 입력이 남아 있습니다")
	}
	if result.Status == "partial" || result.Status == "failed" {
		return fmt.Errorf("oma prep: %s", result.Status)
	}
	return nil
}

const rootHelp = `Usage: oma <command>

Commands:
  prep    Prepare an agent workflow
`

const prepHelp = `Usage: oma prep <JIRA-KEY>
       oma prep --description <text>
       oma prep --empty
`

func main() {
	target, err := runtimechannel.ReleaseExecutable(version, runtimeStatePath, "oma")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "oma:", err)
		os.Exit(1)
	}
	if target != "" {
		if err := syscall.Exec(target, os.Args, os.Environ()); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "oma: release 실행 실패:", err)
			os.Exit(1)
		}
	}

	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			_, _ = fmt.Fprintln(os.Stdout, versionLine("oma", version))
			return
		}
	}

	deps := dependencies{
		IsTerminal: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		Prompter:   &terminalPrompter{input: os.Stdin, output: os.Stderr},
		CandidatesFor: func(repo string) candidateProvider {
			var candidateErr error
			return gitCandidates{repo: repo, runner: gitops.CommandRunner{}, err: &candidateErr}
		},
	}
	if len(os.Args) > 1 && os.Args[1] == "prep" {
		paths := config.ResolvePaths(os.Getenv, "")
		store, storeErr := state.New(paths.StateRoot)
		if storeErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, "oma:", storeErr)
			os.Exit(1)
		}
		defer func() { _ = store.Close() }()
		deps.Workflow = prep.NewPlanner(paths, store, gitops.CommandRunner{}, nil)
	}
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, deps); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
