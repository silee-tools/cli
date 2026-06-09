package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silee-tools/jg/internal/entry"
	"github.com/silee-tools/jg/internal/frecency"
	"github.com/silee-tools/jg/internal/fzf"
	"github.com/silee-tools/jg/internal/scheduler"
	"github.com/silee-tools/jg/internal/shell"
)

var version = "dev"

func main() {
	tool := toolName(os.Args[0])
	args := os.Args[1:]

	if tool == "jgw" {
		runJgw(args)
		return
	}

	if len(args) == 0 {
		runJump(nil)
		return
	}

	switch args[0] {
	case "init":
		runInit(args[1:])
	case "setup":
		runSetup(args[1:])
	case "clean":
		runClean()
	case "scheduler":
		runScheduler(args[1:])
	case "--add", "-add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: jg --add <path>")
			os.Exit(1)
		}
		runAdd(args[1])
	case "--remove", "-remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: jg --remove <path>")
			os.Exit(1)
		}
		runRemove(args[1])
	case "-l", "--list":
		runList()
	case "--clean", "-clean":
		runClean()
	case "-v", "--version", "-version":
		fmt.Fprint(os.Stdout, versionLine("jg", version))
	case "-h", "--help", "-help":
		printHelp()
	default:
		runJump(args)
	}
}

func toolName(argv0 string) string {
	return filepath.Base(argv0)
}

// versionLine 은 모노레포 전 도구가 공유하는 표준 버전 한 줄을 만든다:
// "<도구> v<버전> © 2026 silee-tools".
func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

func printHelp() {
	fmt.Print(`Usage: jg [command] [options]

A frecency-based CLI for quickly jumping to Git repositories.

Commands:
  jg [query...]          Interactive jump with fzf
  jg init <shell>        Output shell integration code (zsh, bash)
  jg setup [shell]       Set up shell integration (auto-detects shell)
  jg clean               Remove stale entries
  jg scheduler <command> Manage daily cleanup scheduler (install, remove, status)

Options:
  --add <path>           Add/update entry for path
  --remove <path>        Remove entry for path
  --clean                Remove stale entries
  -l, --list             List all repos with frecency scores
  -v, --version          Show version
  -h, --help             Show this help
`)
}

// runJgw 는 jgw 모드의 진입점이다. 추후 task 에서 본체를 채운다.
func runJgw(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "-v", "--version", "-version":
			fmt.Fprint(os.Stdout, versionLine("jgw", version))
			return
		case "-h", "--help", "-help":
			printJgwHelp()
			return
		}
	}
	runJgwBody(args)
}

func printJgwHelp() {
	fmt.Print(`Usage: jgw [pattern]

Jump to a worktree of the current git repository or pick a repo first.

Behavior:
  jgw                   In a git repo: pick a worktree. Outside: pick a repo, then a worktree.
  jgw <pattern>         Narrow the repo picker by pattern.

Options:
  -v, --version         Show version
  -h, --help            Show this help
`)
}

func runInit(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: jg init <shell>  (zsh, bash)")
		os.Exit(1)
	}
	code, err := shell.Init(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(code)
}

func runSetup(args []string) {
	var shellOverride string
	if len(args) > 0 {
		shellOverride = args[0]
	}
	result, err := shell.Setup(shellOverride)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if len(result.Actions) == 0 {
		fmt.Fprintf(os.Stderr, "jg is already set up for %s. Nothing to do.\n", result.Shell)
		return
	}
	fmt.Fprintf(os.Stderr, "jg setup complete for %s:\n", result.Shell)
	for _, action := range result.Actions {
		fmt.Fprintf(os.Stderr, "  ✓ %s\n", action)
	}
	fmt.Fprintln(os.Stderr, "\nRestart your shell or run: exec $SHELL")
}

func isSubmodule(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--show-superproject-working-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func runAdd(path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}

	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return
	}

	repoRoot := strings.TrimSpace(string(out))
	if isSubmodule(repoRoot) {
		return
	}
	entry.AddOrUpdate(repoRoot)
}

func runRemove(path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	removed, err := entry.Remove(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if removed {
		fmt.Fprintf(os.Stderr, "Removed: %s\n", absPath)
	} else {
		fmt.Fprintf(os.Stderr, "Not found: %s\n", absPath)
	}
}

func runList() {
	entries, err := entry.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No entries. cd into git repos to start tracking.")
		return
	}

	sorted := frecency.Sort(entries)
	now := time.Now().Unix()
	for _, e := range sorted {
		score := frecency.Score(e.Rank, e.Timestamp, now)
		fmt.Printf("%8.1f  %4.0f  %s\n", score, e.Rank, e.Path)
	}
}

func runClean() {
	report, err := entry.CleanWithReport()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Cleaned %d stale entries", report.Removed)
	if report.Removed > 0 {
		fmt.Fprintf(
			os.Stderr,
			": missing=%d, not-dir=%d, not-git=%d, submodule=%d",
			report.Reasons[entry.ReasonMissing],
			report.Reasons[entry.ReasonNotDirectory],
			report.Reasons[entry.ReasonNotGit],
			report.Reasons[entry.ReasonSubmodule],
		)
	}
	fmt.Fprintln(os.Stderr, ".")
}

func runScheduler(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: jg scheduler <install|remove|status>")
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		result, err := scheduler.Install(os.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Scheduler installed: %s\n", result.PlistPath)
	case "remove":
		removed, err := scheduler.Remove()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if removed {
			fmt.Fprintln(os.Stderr, "Scheduler removed.")
		} else {
			fmt.Fprintln(os.Stderr, "Scheduler is not installed.")
		}
	case "status":
		status, err := scheduler.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if status.Installed {
			fmt.Fprintf(os.Stderr, "Scheduler installed: %s\n", status.PlistPath)
		} else {
			fmt.Fprintf(os.Stderr, "Scheduler not installed: %s\n", status.PlistPath)
		}
	default:
		fmt.Fprintln(os.Stderr, "Usage: jg scheduler <install|remove|status>")
		os.Exit(1)
	}
}

func runJump(queryArgs []string) {
	entries, err := entry.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	valid, _ := entry.FilterValid(entries, entry.ValidatePath)
	if len(valid) != len(entries) {
		_ = entry.Save(valid)
	}

	// 무인자 실행일 때만 현재 저장소의 main working tree 를 최상단에 고정한다.
	var pinnedMain string
	if len(queryArgs) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			pinnedMain = resolvePinnedMain(cwd)
		}
	}

	// 추적 항목이 없고 고정할 main 도 없으면 띄울 것이 없다.
	if len(valid) == 0 && pinnedMain == "" {
		fmt.Fprintln(os.Stderr, "No entries. cd into git repos to start tracking.")
		os.Exit(0)
	}

	query := strings.Join(queryArgs, " ")
	sorted := frecency.SortWithBoost(valid, query)

	selected, err := fzf.Run(sorted, query, pinnedMain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if selected == "" {
		os.Exit(1)
	}

	fmt.Println(selected)
}
