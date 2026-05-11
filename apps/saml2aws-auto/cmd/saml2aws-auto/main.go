// saml2aws-auto: saml2aws AzureAD 로그인 TOTP 자동 주입과 셸 세션 체크.
//
// 전제: AzureAD 가 SSO/cached cookies 로 비밀번호 단계를 건너뛴다.
// PATH 의 totp 바이너리에서 6자리 MFA 코드를 받아 saml2aws login 에 주입한다.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var version = "dev"

const (
	statusExpired     = "expired"
	statusExpiring    = "expiring_soon"
	statusValid       = "valid"
	statusUnknown     = "unknown"
	suppressFileName  = "saml2aws-login-suppress"
	credentialsRel    = ".aws/credentials"
	saml2awsConfigRel = ".saml2aws"
	expiringThreshold = time.Hour
)

type commandRunner interface {
	LookPath(name string) error
	CaptureOutput(name string, args ...string) (string, error)
	Run(name string, args ...string) (int, error)
}

type realRunner struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (r *realRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (r *realRunner) CaptureOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

func (r *realRunner) Run(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}

func resolveTOTPName(autoName, username string) (string, error) {
	if autoName != "" {
		return autoName, nil
	}
	if username == "" {
		return "", errors.New("username not found in SAML2AWS_USERNAME or ~/.saml2aws; SAML2AWS_AUTO_TOTP_NAME not provided")
	}
	return "MS: " + username, nil
}

func resolveUsername(env map[string]string) string {
	if env["SAML2AWS_USERNAME"] != "" {
		return env["SAML2AWS_USERNAME"]
	}
	paths := resolvePaths(env)
	username, _ := readSaml2awsConfigValue(paths.saml2awsConfig, "username")
	return username
}

func readSaml2awsConfigValue(path, key string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == key {
			return strings.TrimSpace(parts[1]), true
		}
	}
	return "", false
}

func runLogin(r commandRunner, env map[string]string, stderr io.Writer) int {
	if err := r.LookPath("saml2aws"); err != nil {
		fmt.Fprintln(stderr, "saml2aws-auto: saml2aws not installed")
		return 127
	}
	if err := r.LookPath("totp"); err != nil {
		fmt.Fprintln(stderr, "saml2aws-auto: totp not installed (PATH 확인)")
		return 127
	}

	totpName, err := resolveTOTPName(env["SAML2AWS_AUTO_TOTP_NAME"], resolveUsername(env))
	if err != nil {
		fmt.Fprintln(stderr, "saml2aws-auto: "+err.Error())
		return 1
	}

	out, err := r.CaptureOutput("totp", totpName)
	if err != nil {
		fmt.Fprintf(stderr, "saml2aws-auto: TOTP unavailable for '%s' (try: totp add \"%s\")\n", totpName, totpName)
		return 1
	}
	code := strings.TrimSpace(out)
	if code == "" {
		fmt.Fprintf(stderr, "saml2aws-auto: TOTP unavailable for '%s' (try: totp add \"%s\")\n", totpName, totpName)
		return 1
	}

	exitCode, err := r.Run("saml2aws", "login",
		"--force",
		"--skip-prompt",
		"--password=",
		"--mfa-token="+code,
	)
	if err != nil {
		fmt.Fprintf(stderr, "saml2aws-auto: failed to invoke saml2aws: %v\n", err)
		return 1
	}
	return exitCode
}

type sessionStatus struct {
	kind    string
	minutes int
}

func checkSession(credentialsPath string, now time.Time) sessionStatus {
	file, err := os.Open(credentialsPath)
	if err != nil {
		return sessionStatus{kind: statusUnknown}
	}
	defer file.Close()

	var expiresRaw string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "x_security_token_expires") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return sessionStatus{kind: statusUnknown}
		}
		expiresRaw = strings.TrimSpace(parts[1])
		break
	}
	if expiresRaw == "" {
		return sessionStatus{kind: statusUnknown}
	}

	expiresAt, err := parseCredentialTime(expiresRaw)
	if err != nil {
		return sessionStatus{kind: statusUnknown}
	}

	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return sessionStatus{kind: statusExpired}
	}
	if remaining <= expiringThreshold {
		return sessionStatus{kind: statusExpiring, minutes: int(remaining / time.Minute)}
	}
	return sessionStatus{kind: statusValid}
}

func parseCredentialTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	normalized := raw
	if len(raw) >= 6 && raw[len(raw)-3] == ':' {
		normalized = raw[:len(raw)-3] + raw[len(raw)-2:]
	}
	return time.Parse("2006-01-02T15:04:05-0700", normalized)
}

type suppressRecord struct {
	typ   string
	value string
	count int
}

func readSuppress(path string) (suppressRecord, bool) {
	file, err := os.Open(path)
	if err != nil {
		return suppressRecord{}, false
	}
	defer file.Close()

	record := suppressRecord{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "type":
			record.typ = parts[1]
		case "value":
			record.value = parts[1]
		case "count":
			fmt.Sscanf(parts[1], "%d", &record.count)
		}
	}
	return record, true
}

func shouldPrompt(suppressPath string, status sessionStatus, now time.Time) bool {
	record, ok := readSuppress(suppressPath)
	if !ok {
		return true
	}

	switch record.typ {
	case "expired":
		return record.value != now.Format("2006-01-02")
	case "expiring":
		if status.kind == statusExpired || record.count >= 2 {
			return true
		}
		_ = writeSuppress(suppressPath, suppressRecord{
			typ:   "expiring",
			value: record.value,
			count: record.count + 1,
		})
		return false
	default:
		return true
	}
}

func writeSuppress(path string, record suppressRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf("type=%s\nvalue=%s\ncount=%d\n", record.typ, record.value, record.count)
	return os.WriteFile(path, []byte(content), 0644)
}

func suppressForStatus(path string, status sessionStatus, now time.Time) error {
	switch status.kind {
	case statusExpired:
		return writeSuppress(path, suppressRecord{typ: "expired", value: now.Format("2006-01-02")})
	case statusExpiring:
		return writeSuppress(path, suppressRecord{typ: "expiring", value: fmt.Sprintf("%d", now.Unix())})
	default:
		return nil
	}
}

type appPaths struct {
	credentials    string
	saml2awsConfig string
	suppress       string
}

func resolvePaths(env map[string]string) appPaths {
	home := env["HOME"]
	if home == "" {
		if resolved, err := os.UserHomeDir(); err == nil {
			home = resolved
		}
	}

	dataHome := env["XDG_DATA_HOME"]
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}

	return appPaths{
		credentials:    filepath.Join(home, credentialsRel),
		saml2awsConfig: filepath.Join(home, saml2awsConfigRel),
		suppress:       filepath.Join(dataHome, suppressFileName),
	}
}

type promptChoice int

const (
	choiceLogin promptChoice = iota + 1
	choiceLater
	choiceSuppress
	choiceCancel
)

func promptLogin(stdin *os.File, stdout io.Writer, status sessionStatus) promptChoice {
	if !term.IsTerminal(int(stdin.Fd())) {
		return choiceLater
	}

	header := ""
	switch status.kind {
	case statusExpired:
		header = "AWS 세션이 만료되었습니다."
	case statusExpiring:
		header = fmt.Sprintf("AWS 세션이 %d분 후 만료됩니다.", status.minutes)
	}

	oldState, err := term.MakeRaw(int(stdin.Fd()))
	if err != nil {
		return choiceLater
	}
	defer term.Restore(int(stdin.Fd()), oldState)

	labels := []string{"지금 로그인", "나중에", "오늘 그만 물어보기"}
	selected := 1
	cReset := "\033[0m"
	cDim := "\033[38;5;244m"
	cHot := "\033[1;7;38;2;255;6;183m"
	cWarn := "\033[1;38;2;255;6;183m"

	render := func() {
		out := ""
		for i, label := range labels {
			n := i + 1
			if n == selected {
				out += fmt.Sprintf(" %s[ %d. %s ]%s", cHot, n, label, cReset)
			} else {
				out += fmt.Sprintf(" %s[ %d. %s ]%s", cDim, n, label, cReset)
			}
		}
		fmt.Fprintf(stdout, "\r\033[K%s", out)
	}

	fmt.Fprintf(stdout, "%s%s%s\r\n", cWarn, header, cReset)
	fmt.Fprint(stdout, "\033[38;5;240m1/2/3 즉시 선택 · ← → 이동 후 Enter 확정 · ESC/q 취소\033[0m\r\n")
	render()

	buf := make([]byte, 1)
	for {
		if _, err := stdin.Read(buf); err != nil {
			fmt.Fprint(stdout, "\r\n")
			return choiceLater
		}
		switch buf[0] {
		case '1', '2', '3':
			fmt.Fprint(stdout, "\r\n")
			return promptChoice(buf[0] - '0')
		case '\r', '\n':
			fmt.Fprint(stdout, "\r\n")
			return promptChoice(selected)
		case 'q', 'Q':
			fmt.Fprint(stdout, "\r\n")
			return choiceCancel
		case 0x1b:
			seq, ok := readEscapeSequence(stdin)
			if !ok {
				fmt.Fprint(stdout, "\r\n")
				return choiceCancel
			}
			switch seq {
			case "[C":
				if selected == 3 {
					selected = 1
				} else {
					selected++
				}
			case "[D":
				if selected == 1 {
					selected = 3
				} else {
					selected--
				}
			default:
				fmt.Fprint(stdout, "\r\n")
				return choiceCancel
			}
		}
		render()
	}
}

func readEscapeSequence(stdin *os.File) (string, bool) {
	fd := int(stdin.Fd())
	buf := make([]byte, 0, 2)
	for len(buf) < 2 {
		if !waitForInput(fd, 50*time.Millisecond) {
			break
		}
		b := make([]byte, 1)
		n, err := stdin.Read(b)
		if err != nil || n == 0 {
			break
		}
		buf = append(buf, b[0])
	}
	return string(buf), len(buf) > 0
}

func waitForInput(fd int, timeout time.Duration) bool {
	var readSet unix.FdSet
	readSet.Set(fd)
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	n, err := unix.Select(fd+1, &readSet, nil, nil, &tv)
	return err == nil && n > 0
}

func runCheck(r commandRunner, env map[string]string, stdin *os.File, stdout, stderr io.Writer, now time.Time) int {
	if err := r.LookPath("saml2aws"); err != nil {
		return 0
	}

	paths := resolvePaths(env)
	status := checkSession(paths.credentials, now)
	switch status.kind {
	case statusValid, statusUnknown:
		return 0
	}
	if !shouldPrompt(paths.suppress, status, now) {
		return 0
	}

	switch promptLogin(stdin, stdout, status) {
	case choiceLogin:
		code := runLogin(r, env, stderr)
		_ = os.Remove(paths.suppress)
		return code
	case choiceSuppress:
		if err := suppressForStatus(paths.suppress, status, now); err != nil {
			fmt.Fprintf(stderr, "saml2aws-auto: failed to write suppress file: %v\n", err)
			return 1
		}
	}
	return 0
}

func envMap(keys ...string) map[string]string {
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = os.Getenv(k)
	}
	return m
}

func usage(stderr io.Writer) {
	fmt.Fprintf(stderr, `Usage: saml2aws-auto <command>

Commands:
  check       Check the current saml2aws session and prompt before expiry.
  login       Run saml2aws login with a TOTP MFA token from the totp binary.
  init zsh    Print zsh plugin setup guidance.

Environment:
  SAML2AWS_USERNAME           saml2aws CLI native — read by both saml2aws
                              and used to compute the default TOTP name.
                              If unset, saml2aws-auto reads username from
                              ~/.saml2aws.
  SAML2AWS_AUTO_TOTP_NAME     Override TOTP entry name. Takes precedence
                              over the SAML2AWS_USERNAME-derived default.

Options:
  -h, --help        Show this help.
  -v, --version     Print version.
`)
}

func printZshInit(stdout, stderr io.Writer, env map[string]string) int {
	paths := resolvePaths(env)
	username := resolveUsername(env)
	if username == "" {
		fmt.Fprintf(stderr, "saml2aws-auto: username not found in SAML2AWS_USERNAME or %s\n", paths.saml2awsConfig)
		fmt.Fprintln(stderr, "saml2aws-auto: run `saml2aws configure` first.")
		return 1
	}

	pluginPath := installedPluginPath(env)
	fmt.Fprintf(stdout, `# saml2aws-auto found username in %s: %s

# zinit
zinit snippet "%s"

# manual source
source "%s"
`, paths.saml2awsConfig, username, pluginPath, pluginPath)
	return 0
}

func installedPluginPath(env map[string]string) string {
	if exe, err := os.Executable(); err == nil {
		if realExe, err := filepath.EvalSymlinks(exe); err == nil {
			prefix := filepath.Dir(filepath.Dir(realExe))
			candidate := filepath.Join(prefix, "share", "saml2aws-auto", "saml2aws-auto.plugin.zsh")
			if strings.Contains(realExe, string(filepath.Separator)+"Cellar"+string(filepath.Separator)+"saml2aws-auto"+string(filepath.Separator)) {
				return candidate
			}
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	dataHome := env["XDG_DATA_HOME"]
	home := env["HOME"]
	if home == "" {
		if resolved, err := os.UserHomeDir(); err == nil {
			home = resolved
		}
	}
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "saml2aws-auto", "saml2aws-auto.plugin.zsh")
}

func main() {
	flags := flag.NewFlagSet("saml2aws-auto", flag.ExitOnError)
	flags.Usage = func() { usage(os.Stderr) }
	showVersion := flags.Bool("version", false, "print version")
	flags.BoolVar(showVersion, "v", false, "print version (shorthand)")
	showHelp := flags.Bool("help", false, "show help")
	flags.BoolVar(showHelp, "h", false, "show help (shorthand)")
	flags.Parse(os.Args[1:])

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *showHelp {
		usage(os.Stderr)
		return
	}
	if flags.NArg() == 0 {
		usage(os.Stderr)
		os.Exit(2)
	}

	r := &realRunner{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	env := envMap("HOME", "XDG_DATA_HOME", "SAML2AWS_USERNAME", "SAML2AWS_AUTO_TOTP_NAME")

	switch flags.Arg(0) {
	case "login":
		os.Exit(runLogin(r, env, os.Stderr))
	case "check":
		os.Exit(runCheck(r, env, os.Stdin, os.Stdout, os.Stderr, time.Now()))
	case "init":
		if flags.NArg() == 2 && flags.Arg(1) == "zsh" {
			os.Exit(printZshInit(os.Stdout, os.Stderr, env))
		}
		fmt.Fprintln(os.Stderr, "Usage: saml2aws-auto init zsh")
		os.Exit(2)
	default:
		usage(os.Stderr)
		os.Exit(2)
	}
}
