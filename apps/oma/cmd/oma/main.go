package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/silee-tools/oma/internal/prep"
	"github.com/silee-tools/oma/internal/runtimechannel"
	"golang.org/x/term"
)

var version = "dev"
var runtimeStatePath string

type candidateProvider interface {
	InputKinds() []promptOption
	Bases() []promptOption
}

type dependencies struct {
	IsTerminal func() bool
	Prompter   Prompter
	Candidates candidateProvider
}

type options struct {
	Input     prep.Input
	PlanToken string
	DryRun    bool
	Yes       bool
	JSON      bool
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
		return completeOptions(&parsed, deps)
	}
	return fmt.Errorf("oma: 지원하지 않는 인자입니다")
}

func completeOptions(parsed *options, deps dependencies) error {
	interactive := deps.IsTerminal != nil && deps.IsTerminal()
	if !interactive {
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
		selected, err := deps.Prompter.Select("기준 브랜치를 선택하세요", deps.Candidates.Bases())
		if err != nil {
			return err
		}
		if selected == "" {
			return errCancelled
		}
		parsed.Input.Base = selected
	}
	approved, err := deps.Prompter.Confirm("이 계획을 적용할까요?")
	if err != nil {
		return err
	}
	if !approved {
		return errCancelled
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
		Prompter:   &terminalPrompter{input: os.Stdin, output: os.Stdout},
	}
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, deps); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
