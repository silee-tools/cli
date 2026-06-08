package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/silee-tools/git-tidy/internal/classify"
	"github.com/silee-tools/git-tidy/internal/gitx"
	"github.com/silee-tools/git-tidy/internal/pick"
	"github.com/silee-tools/git-tidy/internal/reason"
)

var version = "dev"

func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

// effectiveArgs 는 호출된 이름에 따라 실제로 쓸 인자 목록을 돌려준다.
// gtidy! 로 불리면 인자 앞에 --run 을 끼워 넣고, 그 외에는 인자를 그대로 둔다.
func effectiveArgs(invoked string, args []string) []string {
	if invoked == "gtidy!" {
		return append([]string{"--run"}, args...)
	}
	return args
}

type options struct {
	run       bool
	noTUI     bool
	noFetch   bool
	staleDays int
}

func defaultStaleDays() int {
	if v := os.Getenv("GIT_TIDY_STALE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 20
}

// parseArgs 는 인자를 옵션으로 바꾼다. -v/-h 는 상위에서 먼저 처리한다.
func parseArgs(args []string) (options, error) {
	o := options{staleDays: defaultStaleDays()}
	for _, a := range args {
		switch {
		case a == "--run":
			o.run = true
		case a == "--no-tui":
			o.noTUI = true
		case a == "--no-fetch":
			o.noFetch = true
		case len(a) > 13 && a[:13] == "--stale-days=":
			n, err := strconv.Atoi(a[13:])
			if err != nil || n <= 0 {
				return o, fmt.Errorf("잘못된 --stale-days 값: %s", a[13:])
			}
			o.staleDays = n
		default:
			return o, fmt.Errorf("알 수 없는 옵션: %s", a)
		}
	}
	return o, nil
}

const helpText = `Usage: git-tidy [--run] [options]

작업이 끝났거나 오래 방치된 로컬 git 브랜치를 정리한다.

  git-tidy              dry-run — 삭제 대상만 표시
  git-tidy --run        삭제 대상을 다중 선택해 삭제
  git-tidy --run --no-tui  체크박스 TUI 대신 줄 기반 선택
  --stale-days=N        stale 판정 창 (기본 20, GIT_TIDY_STALE_DAYS)
  --no-fetch            git fetch --prune 건너뛰기
  -v, --version         버전 출력
  -h, --help            도움말 출력

단축 명령:
  gtidy                 git-tidy 와 동일
  gtidy!                git-tidy --run 과 동일
`

func main() {
	invoked := filepath.Base(os.Args[0])
	args := effectiveArgs(invoked, os.Args[1:])

	for _, a := range args {
		switch a {
		case "-v", "--version":
			_, _ = fmt.Fprint(os.Stdout, versionLine(invoked, version))
			return
		case "-h", "--help":
			fmt.Print(helpText)
			return
		}
	}

	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-tidy:", err)
		fmt.Fprintln(os.Stderr, "git-tidy --help 로 사용법을 확인하세요.")
		os.Exit(1)
	}
	os.Exit(run(opts))
}

// run 은 git-tidy 본체다. 종료 코드를 돌려준다.
func run(opts options) int {
	if !gitx.IsRepo() {
		fmt.Fprintln(os.Stderr, "git-tidy: git 저장소가 아닙니다.")
		return 1
	}
	if !opts.noFetch {
		gitx.FetchPrune()
	}

	result, broken, err := buildClassification(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-tidy:", err)
		return 1
	}
	warnBrokenBranches(broken)

	if len(result.ToDelete) == 0 {
		fmt.Println("정리할 브랜치가 없습니다.")
		return 0
	}
	if !opts.run {
		// dry-run 일 때만 목록을 출력한다. --run 이면 선택 화면이 같은 목록을
		// 그리므로 여기서 출력하면 화면이 중복된다.
		printTargets(result)
		fmt.Println("\n→ git-tidy --run 으로 삭제를 진행하세요.")
		return 0
	}
	return runDeletion(result, opts)
}

// warnBrokenBranches 는 객체가 유실돼 분류할 수 없는 브랜치를 사용자에게 알린다.
// 이런 브랜치는 git 저장소 손상의 신호이므로 자동 삭제하지 않고 안내만 한다.
func warnBrokenBranches(broken []string) {
	if len(broken) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "경고: 객체가 유실돼 분류할 수 없는 브랜치 %d개 (저장소 손상 가능):\n", len(broken))
	for _, name := range broken {
		fmt.Fprintf(os.Stderr, "  %s\n", name)
	}
	fmt.Fprintln(os.Stderr, "  → git fsck 로 점검하세요. git-tidy 는 이 브랜치들을 건드리지 않습니다.")
}

func buildClassification(opts options) (classify.Classified, []string, error) {
	branches, broken, err := gitx.LocalBranches()
	if err != nil {
		return classify.Classified{}, nil, err
	}
	base := gitx.BaseBranch()
	merged, err := gitx.MergedBranches(base)
	if err != nil {
		return classify.Classified{}, nil, err
	}
	worktrees, err := gitx.WorktreeBranches()
	if err != nil {
		return classify.Classified{}, nil, err
	}
	baseCommits, err := gitx.BaseCommits(base)
	if err != nil {
		return classify.Classified{}, nil, err
	}
	in := classify.Input{
		Now:         time.Now().Unix(),
		StaleDays:   opts.staleDays,
		Base:        base,
		Current:     gitx.CurrentBranch(),
		Branches:    branches,
		Merged:      merged,
		Worktrees:   worktrees,
		BaseCommits: baseCommits,
		MergeBaseUnix: func(branch string) (int64, bool) {
			return gitx.MergeBaseUnix(base, branch)
		},
	}
	return classify.Classify(in), broken, nil
}

// printTargets 는 c.ToDelete 가 신호 순(gone → merged → absorbed → stale)으로 정렬돼 있다고 가정한다.
// classify.Classify 가 이 순서를 보장하며, 그래야 그룹 헤더가 신호당 한 번만 출력된다.
func printTargets(c classify.Classified) {
	printTargetsTo(os.Stdout, c)
}

func printTargetsTo(out io.Writer, c classify.Classified) {
	_, _ = fmt.Fprintf(out, "삭제 대상 (%d):\n", len(c.ToDelete))
	var cur classify.Signal
	for _, r := range c.ToDelete {
		if r.Signal != cur {
			cur = r.Signal
			_, _ = fmt.Fprintf(out, "  [%s]\n", cur)
			if desc := reason.Description(string(cur)); desc != "" {
				_, _ = fmt.Fprintf(out, "    %s\n", desc)
			}
		}
		line := "    " + r.Name
		if r.WorktreePath != "" {
			line += "  [worktree 동반 제거: " + filepath.Base(r.WorktreePath) + "]"
		}
		if r.AgeDays > 0 {
			line += fmt.Sprintf("  (%d일 경과)", r.AgeDays)
		}
		if r.Signal == classify.SignalAbsorbed && r.AbsorbedByShortHash != "" {
			line += "  (base: " + r.AbsorbedByShortHash
			if r.AbsorbedBySubject != "" {
				line += " " + r.AbsorbedBySubject
			}
			line += ")"
		}
		_, _ = fmt.Fprintln(out, line)
	}
	if len(c.Excluded) > 0 {
		_, _ = fmt.Fprintf(out, "제외된 후보 (%d):\n", len(c.Excluded))
		for _, e := range c.Excluded {
			_, _ = fmt.Fprintf(out, "  %s  (%s)  [보호: %s]\n", e.Name, e.Signal, e.Reason)
		}
	}
	if c.OtherCount > 0 {
		_, _ = fmt.Fprintf(out, "그 외 브랜치 %d개는 정리 대상이 아닙니다.\n", c.OtherCount)
	}
}

// runDeletion 은 --run 경로다. 다중 선택을 거쳐 선택된 브랜치를 삭제한다.
func runDeletion(c classify.Classified, opts options) int {
	items := make([]pick.Item, len(c.ToDelete))
	byName := map[string]classify.Result{}
	for i, r := range c.ToDelete {
		items[i] = pick.Item{
			Name:                r.Name,
			Signal:              string(r.Signal),
			WorktreePath:        r.WorktreePath,
			AgeDays:             r.AgeDays,
			AbsorbedByShortHash: r.AbsorbedByShortHash,
			AbsorbedBySubject:   r.AbsorbedBySubject,
			Checked:             r.Signal == classify.SignalGone, // gone 만 기본 체크, merged·stale 은 사용자가 직접 선택
		}
		byName[r.Name] = r
	}
	sel := pick.NewSelection(items)

	var chosen []string
	var ok bool
	switch pick.DetectMode(opts.noTUI) {
	case pick.ModeNone:
		fmt.Fprintln(os.Stderr, "git-tidy: 삭제하려면 터미널이 필요합니다. 목록은 인자 없는 git-tidy 로 확인하세요.")
		return 1
	case pick.ModeTUI:
		var fellBack bool
		chosen, ok, fellBack = pick.RunTUI(sel)
		if fellBack {
			chosen, ok = pick.RunLine(sel, os.Stdin, os.Stdout)
		}
	case pick.ModeLine:
		chosen, ok = pick.RunLine(sel, os.Stdin, os.Stdout)
	}
	if !ok || len(chosen) == 0 {
		fmt.Println("삭제하지 않았습니다.")
		return 0
	}
	return deleteBranches(chosen, byName)
}

func deleteBranches(chosen []string, byName map[string]classify.Result) int {
	failed := 0
	for _, name := range chosen {
		r := byName[name]
		if r.WorktreePath != "" {
			if err := gitx.RemoveWorktree(r.WorktreePath); err != nil {
				fmt.Printf("  실패: %s (worktree 정리 안 됨)\n", name)
				failed++
				continue
			}
		}
		if err := gitx.DeleteBranch(name); err != nil {
			fmt.Printf("  실패: %s\n", name)
			failed++
			continue
		}
		fmt.Printf("  삭제됨: %s\n", name)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
