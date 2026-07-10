// Command totp is a macOS Keychain-backed TOTP code generator. It mirrors
// the original totp.plugin.zsh function 1:1.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/silee-tools/cli/apps/totp/internal/code"
	"github.com/silee-tools/cli/apps/totp/internal/runtimechannel"
	"github.com/silee-tools/cli/apps/totp/internal/store"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"
var runtimeStatePath string

const helpText = `totp %s — macOS Keychain 기반 TOTP 생성기

Usage:
  totp [FLAGS] [SUBCOMMAND] [ARGS...]

Flags:
  -h, --help            도움말 출력 후 종료
  -v, --version         버전 출력 후 종료

Subcommands:
  (none)                fzf picker (totp 마커 항목만) → 코드 출력
  <name>                6자리 코드 출력 + 클립보드 복사
  add <name>            secret 등록 + totp 마커 (입력 숨김)
  rm <name>             등록 제거 (alias: remove, delete)
  ls [--all] [pattern]  마커 항목 나열. --all은 마커 무시 (alias: list)
  tag <name>            기존 keychain 항목에 totp 마커 부착 (마이그레이션)
  help                  이 도움말 출력

저장 컨벤션:
  service="<name>"  account=$USER  kind=%q

예:
  totp add "MS: you@example.com"
  totp     "MS: you@example.com"
  totp ls
  totp ls --all "MS:"
  totp tag "MS: you@example.com"
`

func main() {
	target, err := runtimechannel.ReleaseExecutable(version, runtimeStatePath, "totp")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "totp:", err)
		os.Exit(1)
	}
	if target != "" {
		if err := syscall.Exec(target, os.Args, os.Environ()); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "totp: release 실행 실패:", err)
			os.Exit(1)
		}
	}

	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		// Special exit code 130 mirrors the zsh version (fzf cancellation).
		var ec exitCodeError
		if errors.As(err, &ec) {
			if ec.msg != "" {
				fmt.Fprintln(os.Stderr, ec.msg)
			}
			os.Exit(ec.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type exitCodeError struct {
	code int
	msg  string
}

func (e exitCodeError) Error() string { return e.msg }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runPicker(stdout, stderr)
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, helpText, version, store.Marker)
		return nil
	case "-v", "--version":
		fmt.Fprintf(stdout, "totp v%s © 2026 silee-tools\n", version)
		return nil
	case "add":
		return runAdd(args[1:], stdin, stdout, stderr)
	case "rm", "remove", "delete":
		return runRemove(args[1:], stdout)
	case "tag":
		return runTag(args[1:], stdout)
	case "ls", "list":
		return runList(args[1:], stdout)
	default:
		return runGenerate(args[0], stdout, stderr)
	}
}

func openStore() (store.Store, error) {
	return store.NewKeychain()
}

func runAdd(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) < 1 || args[0] == "" {
		return exitCodeError{code: 2, msg: "usage: totp add <name>"}
	}
	name := args[0]
	secret, err := readSecret(stdin, stderr, fmt.Sprintf("TOTP secret for %s (input hidden): ", name))
	if err != nil {
		return err
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return exitCodeError{code: 1, msg: "totp: empty secret, aborted"}
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	if err := st.Add(name, secret); err != nil {
		return fmt.Errorf("totp: keychain write failed: %w", err)
	}
	fmt.Fprintf(stdout, "totp: stored %q\n", name)
	return nil
}

func runRemove(args []string, stdout io.Writer) error {
	if len(args) < 1 || args[0] == "" {
		return exitCodeError{code: 2, msg: "usage: totp rm <name>"}
	}
	name := args[0]
	st, err := openStore()
	if err != nil {
		return err
	}
	if err := st.Remove(name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return exitCodeError{code: 1, msg: fmt.Sprintf("totp: %q not found", name)}
		}
		return err
	}
	fmt.Fprintf(stdout, "totp: removed %q\n", name)
	return nil
}

func runTag(args []string, stdout io.Writer) error {
	if len(args) < 1 || args[0] == "" {
		return exitCodeError{code: 2, msg: "usage: totp tag <name>  (기존 항목에 totp 마커 부착)"}
	}
	name := args[0]
	st, err := openStore()
	if err != nil {
		return err
	}
	if err := st.Tag(name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return exitCodeError{code: 1, msg: fmt.Sprintf("totp: %q not found in keychain", name)}
		}
		return fmt.Errorf("totp: keychain update failed: %w", err)
	}
	fmt.Fprintf(stdout, "totp: tagged %q\n", name)
	return nil
}

func runList(args []string, stdout io.Writer) error {
	markedOnly := true
	pattern := ""
	for _, a := range args {
		if a == "--all" {
			markedOnly = false
			continue
		}
		pattern = a
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	names, err := st.List(markedOnly, pattern)
	if err != nil {
		return err
	}
	for _, n := range names {
		fmt.Fprintln(stdout, n)
	}
	return nil
}

func runGenerate(name string, stdout, stderr io.Writer) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	secret, err := st.Get(name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return exitCodeError{
				code: 1,
				msg:  fmt.Sprintf("totp: %q not found in keychain (try: totp add %q)", name, name),
			}
		}
		return err
	}
	codeStr, err := code.Generate(secret)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, codeStr)
	copyToClipboard(codeStr) // best-effort
	return nil
}

func runPicker(stdout, stderr io.Writer) error {
	if _, err := exec.LookPath("fzf"); err != nil {
		return exitCodeError{code: 127, msg: "totp: fzf not installed (인터랙티브 모드 필요)"}
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	names, err := st.List(true, "")
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return exitCodeError{code: 130, msg: "totp: no marked entries (try: totp tag <name> or totp add <name>)"}
	}
	cmd := exec.Command("fzf",
		"--prompt=totp> ",
		"--height=40%",
		"--reverse",
		"--no-multi",
		"--header=totp 마커 항목 (없으면: totp tag <name>)",
	)
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		// fzf returns 130 on cancel.
		return exitCodeError{code: 130}
	}
	picked := strings.TrimSpace(string(out))
	if picked == "" {
		return exitCodeError{code: 130}
	}
	return runGenerate(picked, stdout, stderr)
}

// readSecret reads a single line of input. If stdin is a terminal, it
// reads with echo off; otherwise it reads from the provided reader.
func readSecret(stdin io.Reader, stderr io.Writer, prompt string) (string, error) {
	fmt.Fprint(stderr, prompt)
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr) // newline after hidden input
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// Non-terminal stdin (pipe, test): read one line.
	r := bufio.NewReader(stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// copyToClipboard pipes s into pbcopy. Best-effort: silently skipped if
// pbcopy is unavailable (non-macOS or stripped environments).
func copyToClipboard(s string) {
	if _, err := exec.LookPath("pbcopy"); err != nil {
		return
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	_ = cmd.Run()
}
