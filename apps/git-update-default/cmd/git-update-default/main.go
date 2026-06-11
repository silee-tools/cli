package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silee-tools/git-update-default/internal/confirm"
	"github.com/silee-tools/git-update-default/internal/gitx"
	"github.com/silee-tools/git-update-default/internal/resolve"
)

var version = "dev"

func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

const helpText = `Usage: git-update-default [--stash | --force]

현재 위치가 속한 git 저장소를 원격 default branch 의 최신 상태로 전환한다.
저장소 안 어느 하위 경로에서 실행해도 동작한다.

  git-update-default          default branch 로 전환하고 최신까지 fast-forward
  --stash                     dirty 일 때 묻지 않고 stash 후 진행
  --force                     dirty 일 때 묻지 않고 추적 변경을 버리고 진행
  -v, --version               버전 출력
  -h, --help                  도움말 출력

dirty 인 채로 인터랙티브하게 실행하면 변경 파일 목록을 보여주고
stash 후 진행 / force / 취소(기본값) 를 고를 수 있다.
`

func main() {
	invoked := filepath.Base(os.Args[0])
	args := os.Args[1:]

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
		fmt.Fprintln(os.Stderr, "git-update-default:", err)
		fmt.Fprintln(os.Stderr, "git-update-default --help 로 사용법을 확인하세요.")
		os.Exit(1)
	}
	os.Exit(run(opts))
}

type options struct {
	stash bool
	force bool
}

func parseArgs(args []string) (options, error) {
	o := options{}
	for _, a := range args {
		switch a {
		case "--stash":
			o.stash = true
		case "--force":
			o.force = true
		default:
			return o, fmt.Errorf("알 수 없는 옵션: %s", a)
		}
	}
	return o, nil
}

// dirtyAction 은 dirty 작업 트리를 만났을 때 따라갈 경로다.
type dirtyAction int

const (
	pathInteractive dirtyAction = iota // TUI 로 묻는다
	pathStash                          // 묻지 않고 stash
	pathForce                          // 묻지 않고 추적 변경 폐기
	pathCancel                         // 묻지 않고 멈춘다
)

// dirtyPath 는 환경(TTY 여부)과 플래그로 dirty 처리 경로를 정한다.
// 플래그(--force/--stash)가 있으면 TTY 여도 묻지 않는다. force 가 stash 보다 우선한다.
// 플래그가 없으면 TTY 일 때만 인터랙티브로 묻고, 비-TTY 면 취소로 안전하게 멈춘다.
func dirtyPath(tty, stash, force bool) dirtyAction {
	switch {
	case force:
		return pathForce
	case stash:
		return pathStash
	case tty:
		return pathInteractive
	default:
		return pathCancel
	}
}

// dirtyResult 는 dirty 처리 후 run 이 어떻게 이어갈지를 나타낸다.
type dirtyResult int

const (
	dirtyProceed  dirtyResult = iota // 처리 끝 — 전환·최신화로 계속
	dirtyAbortOK                     // 사용자 취소 — 종료 코드 0
	dirtyAbortErr                    // 처리 수단 없음 또는 처리 실패 — 종료 코드 1
)

// run 은 git-update-default 본체다. 종료 코드를 돌려준다.
func run(opts options) int {
	if !gitx.IsRepo() {
		fmt.Fprintln(os.Stderr, "git-update-default: git 저장소가 아닙니다.")
		return 1
	}
	if !gitx.HasOriginRemote() {
		fmt.Fprintln(os.Stderr, "git-update-default: origin 원격이 없어 default branch 를 정할 수 없습니다.")
		return 1
	}
	if err := gitx.FetchPrune(); err != nil {
		// fetch 실패(오프라인 등)는 치명적이지 않다. 로컬에 이미 있는 원격 추적
		// 참조로 진행하되, 최신이 아닐 수 있음을 알린다.
		fmt.Fprintln(os.Stderr, "경고: git fetch 실패 — 로컬의 원격 추적 정보로 진행합니다.")
	}

	branch, ok := resolve.Default(resolve.Deps{
		RemoteBranchExists: gitx.RemoteBranchExists,
		GitHubDefault:      gitx.GitHubDefault,
		SymbolicRef:        gitx.SymbolicRefDefault,
	})
	if !ok {
		fmt.Fprintln(os.Stderr, "git-update-default: default branch 를 정할 수 없습니다 (main·master·gh·origin/HEAD 모두 실패).")
		return 1
	}

	files, err := gitx.DirtyFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-update-default:", err)
		return 1
	}
	if len(files) > 0 {
		switch handleDirty(files, opts) {
		case dirtyAbortOK:
			return 0
		case dirtyAbortErr:
			return 1
		}
	}

	if code := switchTo(branch); code != 0 {
		return code
	}

	if err := gitx.MergeFFOnly(branch); err != nil {
		fmt.Fprintf(os.Stderr, "git-update-default: %s 가 origin/%s 와 갈라져 fast-forward 할 수 없습니다.\n", branch, branch)
		fmt.Fprintln(os.Stderr, "  → 직접 rebase·reset 으로 정리하세요. 강제로 맞추지 않습니다.")
		return 1
	}

	fmt.Printf("✓ %s 로 전환하고 origin/%s 최신까지 맞췄습니다.\n", branch, branch)
	return 0
}

// handleDirty 는 dirty 작업 트리를 처리하고, run 이 이어갈 방향을 돌려준다.
// 인터랙티브 취소는 사용자가 정상적으로 무른 것이므로 dirtyAbortOK(종료 코드 0)로,
// 비-TTY 에서 처리 수단이 없어 멈추거나 stash·force 가 실패하면 dirtyAbortErr(종료
// 코드 1)로 돌려준다. 처리에 성공해 계속 진행할 때만 dirtyProceed 다.
func handleDirty(files []string, opts options) dirtyResult {
	action := confirm.ActionCancel
	switch dirtyPath(confirm.IsTTY(), opts.stash, opts.force) {
	case pathInteractive:
		action = confirm.Run(files)
	case pathStash:
		action = confirm.ActionStash
	case pathForce:
		action = confirm.ActionForce
	case pathCancel:
		printDirty(files)
		fmt.Fprintln(os.Stderr, "git-update-default: 커밋되지 않은 변경이 있습니다. --stash 또는 --force 를 쓰거나 직접 정리하세요.")
		return dirtyAbortErr
	}

	switch action {
	case confirm.ActionCancel:
		fmt.Println("취소했습니다. 아무것도 바꾸지 않았습니다.")
		return dirtyAbortOK
	case confirm.ActionStash:
		if err := gitx.StashPush(); err != nil {
			fmt.Fprintln(os.Stderr, "git-update-default: stash 실패:", err)
			return dirtyAbortErr
		}
		cur := gitx.CurrentBranch()
		fmt.Printf("변경을 stash 했습니다. 원래 브랜치(%s)로 돌아가 `git stash pop` 으로 복원하세요.\n", cur)
	case confirm.ActionForce:
		if err := gitx.ResetHard(); err != nil {
			fmt.Fprintln(os.Stderr, "git-update-default: reset 실패:", err)
			return dirtyAbortErr
		}
		fmt.Println("추적 변경을 버렸습니다.")
	}
	return dirtyProceed
}

// printDirty 는 변경 파일 목록을 그대로 출력한다(비-TTY·취소 안내용).
func printDirty(files []string) {
	fmt.Fprintf(os.Stderr, "커밋되지 않은 변경 %d개:\n", len(files))
	for _, f := range files {
		fmt.Fprintln(os.Stderr, "  "+f)
	}
}

// switchTo 는 default branch 로 전환한다. 이미 그 브랜치면 아무것도 하지 않는다.
func switchTo(branch string) int {
	if gitx.CurrentBranch() == branch {
		return 0
	}
	var err error
	if gitx.LocalBranchExists(branch) {
		err = gitx.Switch(branch)
	} else {
		err = gitx.SwitchCreateTracking(branch)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-update-default: 브랜치 전환 실패:", err)
		return 1
	}
	return 0
}
