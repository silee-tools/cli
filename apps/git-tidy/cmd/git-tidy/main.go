package main

import (
	"fmt"
	"os"
)

var version = "dev"

// versionLine 은 모노레포 전 도구가 공유하는 표준 버전 한 줄을 만든다.
func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "-v", "--version":
			_, _ = fmt.Fprint(os.Stdout, versionLine("git-tidy", version))
			return
		case "-h", "--help":
			fmt.Print(helpText)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "git-tidy: not implemented yet")
}

const helpText = `Usage: git-tidy [--run] [options]

Clean up local git branches that are done or stale.
`
