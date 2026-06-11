package main

import (
	"fmt"
	"os"
	"path/filepath"
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

func run(opts options) int {
	_ = opts
	fmt.Fprintln(os.Stderr, "git-update-default: 아직 구현되지 않았습니다.")
	return 1
}
