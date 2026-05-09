// saml2aws-auto-login: saml2aws AzureAD 로그인 TOTP 자동 주입.
//
// 전제: AzureAD 가 SSO/cached cookies 로 비밀번호 단계를 건너뛴다.
// PATH 의 totp 바이너리에서 6자리 MFA 코드를 받아 saml2aws login 에 주입한다.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var version = "dev"

// commandRunner 는 외부 프로세스 실행을 추상화하여 테스트에서 mock 가능하게 한다.
type commandRunner interface {
	// LookPath 는 PATH 에서 바이너리를 찾는다. 없으면 error.
	LookPath(name string) error
	// CaptureOutput 은 stdout 을 캡처해 반환한다. stderr 는 무시한다.
	CaptureOutput(name string, args ...string) (string, error)
	// Run 은 stdin/stdout/stderr 를 그대로 inherit 시키고 exit code 를 반환한다.
	Run(name string, args ...string) (int, error)
}

// realRunner 는 실제 OS 프로세스를 실행한다.
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

// resolveTOTPName 은 환경변수에서 TOTP 항목 이름을 결정한다.
//
// 우선순위:
//  1. SAML2AWS_AUTO_TOTP_NAME 이 비어있지 않으면 그대로
//  2. SAML2AWS_USERNAME 이 있으면 "MS: ${SAML2AWS_USERNAME}"
//  3. 둘 다 없으면 에러
func resolveTOTPName(autoName, username string) (string, error) {
	if autoName != "" {
		return autoName, nil
	}
	if username == "" {
		return "", errors.New("SAML2AWS_USERNAME unset and SAML2AWS_AUTO_TOTP_NAME not provided")
	}
	return "MS: " + username, nil
}

// run 은 메인 로직을 실행하고 종료 코드를 반환한다. 테스트 가능하게 분리.
func run(r commandRunner, env map[string]string, stderr io.Writer) int {
	if err := r.LookPath("saml2aws"); err != nil {
		fmt.Fprintln(stderr, "saml2aws-auto: saml2aws not installed")
		return 127
	}
	if err := r.LookPath("totp"); err != nil {
		fmt.Fprintln(stderr, "saml2aws-auto: totp not installed (PATH 확인)")
		return 127
	}

	totpName, err := resolveTOTPName(env["SAML2AWS_AUTO_TOTP_NAME"], env["SAML2AWS_USERNAME"])
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

func envMap(keys ...string) map[string]string {
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = os.Getenv(k)
	}
	return m
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: saml2aws-auto-login

Automate saml2aws AzureAD login by injecting a TOTP MFA code from the
'totp' binary on PATH. Assumes AzureAD SSO/cached cookies skip the
password step.

Environment:
  SAML2AWS_USERNAME           saml2aws CLI native — read by both saml2aws
                              and used to compute the default TOTP name
                              ("MS: $SAML2AWS_USERNAME").
  SAML2AWS_AUTO_TOTP_NAME     Override TOTP entry name. Takes precedence
                              over the SAML2AWS_USERNAME-derived default.

Options:
  -h, --help        Show this help.
  -v, --version     Print version.
`)
	}
	showVersion := flag.Bool("version", false, "print version")
	flag.BoolVar(showVersion, "v", false, "print version (shorthand)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	r := &realRunner{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	env := envMap("SAML2AWS_USERNAME", "SAML2AWS_AUTO_TOTP_NAME")
	os.Exit(run(r, env, os.Stderr))
}
