package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/silee-tools/mydesk/cmd"
	"github.com/silee-tools/mydesk/internal/ui"
)

var version = "dev"

func main() {
	var opts cmd.GlobalOpts

	flag.BoolVar(&opts.DryRun, "dry-run", false, "show what would be done without making changes")
	flag.BoolVar(&opts.Verbose, "verbose", false, "show detailed output")
	flag.BoolVar(&opts.NoColor, "no-color", false, "disable colored output")
	flag.StringVar(&opts.ConfigDir, "config-dir", "", "path to config repository")
	flag.Usage = usage
	flag.Parse()

	ui.Init(opts.NoColor)

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "init":
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		err = cmd.RunInit(opts, target)
	case "link":
		err = cmd.RunLink(opts)
	case "unlink":
		err = cmd.RunUnlink(opts)
	case "diff":
		err = cmd.RunDiff(opts)
	case "sync":
		err = cmd.RunSync(opts)
	case "setup":
		err = cmd.RunSetup(opts)
	case "install-shell":
		err = cmd.RunInstallShell(opts)
	case "web":
		port := 8080
		noOpen := false
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--no-open":
				noOpen = true
			case args[i] == "--port" && i+1 < len(args):
				i++
				if _, scanErr := fmt.Sscanf(args[i], "%d", &port); scanErr != nil {
					ui.Error("invalid port: %s", args[i])
					os.Exit(1)
				}
			}
		}
		err = cmd.RunWeb(opts, port, noOpen, resolveVersion())
	case "version":
		fmt.Printf("mydesk %s\n", resolveVersion())
	case "help":
		usage()
	default:
		ui.Error("unknown command: %s", args[0])
		usage()
		os.Exit(1)
	}

	if err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
}

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func usage() {
	fmt.Fprintf(os.Stderr, `mydesk - macOS config backup & sync tool (Mackup alternative)

Usage:
  mydesk [flags] <command> [args]

Commands:
  init <dir>     Create a new config repository scaffold
  link           Create symlinks (native dirs + links.conf)
  unlink         Remove symlinks and restore backups
  diff           Detect drift between system and config repo
  sync           Export current system state to config repo
  setup          Full provisioning (brew, omz, mise, link, defaults, vscode)
  install-shell  Write MYDESK_CONFIG_DIR to shell profile
  web            Start web UI (--port 8080 --no-open)
  version        Print version
  help           Show this help

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Native directories (auto-detected in config repo):
  home/     → ~/           (symlink each file)
  config/   → ~/.config/   (symlink each file)
  ssh/      → ~/.ssh/      (symlink each file)
  vscode/   → VS Code User dir (symlink + extensions)
  brew/     → Brewfile sync/install
  macos/    → defaults.sh execution
  omz/      → Oh-My-Zsh setup

Environment:
  MYDESK_CONFIG_DIR   Path to config repository
  MYDESK_REPOS        Base path for $REPOS variable (default: ~/Repositories)
  NO_COLOR            Disable colored output
`)
}
