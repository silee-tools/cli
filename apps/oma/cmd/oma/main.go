package main

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/silee-tools/oma/internal/runtimechannel"
)

var version = "dev"
var runtimeStatePath string

type dependencies struct{}

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
	return fmt.Errorf("oma: 지원하지 않는 인자입니다")
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

	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, dependencies{}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
