package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeRunner 는 commandRunner 를 mock 한다.
type fakeRunner struct {
	missing      map[string]bool // LookPath 에서 not found 처리할 바이너리
	captureOut   string          // CaptureOutput stdout
	captureErr   error           // CaptureOutput error
	captureCalls [][]string      // [name, args...] 기록
	runExit      int             // Run exit code
	runErr       error           // Run invocation error
	runCalls     [][]string      // [name, args...] 기록
}

func (f *fakeRunner) LookPath(name string) error {
	if f.missing[name] {
		return errors.New("not found: " + name)
	}
	return nil
}

func (f *fakeRunner) CaptureOutput(name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.captureCalls = append(f.captureCalls, call)
	return f.captureOut, f.captureErr
}

func (f *fakeRunner) Run(name string, args ...string) (int, error) {
	call := append([]string{name}, args...)
	f.runCalls = append(f.runCalls, call)
	return f.runExit, f.runErr
}

func TestResolveTOTPName(t *testing.T) {
	tests := []struct {
		name     string
		autoName string
		username string
		want     string
		wantErr  bool
	}{
		{"explicit override", "Custom Entry", "alice", "Custom Entry", false},
		{"override even when username empty", "Custom Entry", "", "Custom Entry", false},
		{"default from username", "", "alice", "MS: alice", false},
		{"both empty errors", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTOTPName(tt.autoName, tt.username)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestResolveUsername(t *testing.T) {
	home := t.TempDir()
	config := []byte("[default]\nusername                = alice@example.com\n")
	if err := os.WriteFile(filepath.Join(home, ".saml2aws"), config, 0644); err != nil {
		t.Fatal(err)
	}

	if got := resolveUsername(map[string]string{"HOME": home}); got != "alice@example.com" {
		t.Fatalf("got %q", got)
	}
	if got := resolveUsername(map[string]string{
		"HOME":              home,
		"SAML2AWS_USERNAME": "env@example.com",
	}); got != "env@example.com" {
		t.Fatalf("env override got %q", got)
	}
}

func TestRun_MissingSaml2aws(t *testing.T) {
	r := &fakeRunner{missing: map[string]bool{"saml2aws": true}}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr, "")
	if code != 127 {
		t.Fatalf("exit=%d want 127", code)
	}
	if !strings.Contains(stderr.String(), "saml2aws not installed") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_MissingTotp(t *testing.T) {
	r := &fakeRunner{missing: map[string]bool{"totp": true}}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr, "")
	if code != 127 {
		t.Fatalf("exit=%d want 127", code)
	}
	if !strings.Contains(stderr.String(), "totp not installed") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_MissingUsernameAndOverride(t *testing.T) {
	r := &fakeRunner{}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"HOME": t.TempDir()}, &stderr, "")
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "username not found") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_TOTPFailure(t *testing.T) {
	r := &fakeRunner{captureErr: errors.New("no entry")}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr, "")
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "TOTP unavailable for 'MS: alice'") {
		t.Fatalf("stderr=%q", got)
	}
	if !strings.Contains(got, `try: totp add "MS: alice"`) {
		t.Fatalf("stderr missing hint: %q", got)
	}
}

func TestRun_TOTPEmptyOutput(t *testing.T) {
	r := &fakeRunner{captureOut: "   \n"}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr, "")
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
}

func TestRun_HappyPath(t *testing.T) {
	r := &fakeRunner{captureOut: "123456\n", runExit: 0}
	var stderr bytes.Buffer
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".saml2aws"), []byte("username = alice\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code := runLogin(r, map[string]string{"HOME": home}, &stderr, "")
	if code != 0 {
		t.Fatalf("exit=%d want 0, stderr=%q", code, stderr.String())
	}
	if len(r.captureCalls) != 1 || r.captureCalls[0][0] != "totp" || r.captureCalls[0][1] != "MS: alice" {
		t.Fatalf("captureCalls=%v", r.captureCalls)
	}
	if len(r.runCalls) != 1 {
		t.Fatalf("runCalls=%v", r.runCalls)
	}
	got := r.runCalls[0]
	want := []string{"saml2aws", "login", "--force", "--skip-prompt", "--password=", "--mfa-token=123456"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestRun_UsesSessionDurationFromSaml2awsConfig(t *testing.T) {
	r := &fakeRunner{captureOut: "123456\n", runExit: 0}
	var stderr bytes.Buffer
	home := t.TempDir()
	config := "username = alice\naws_session_duration = 43200\n"
	if err := os.WriteFile(filepath.Join(home, ".saml2aws"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	code := runLogin(r, map[string]string{"HOME": home}, &stderr, "")
	if code != 0 {
		t.Fatalf("exit=%d want 0, stderr=%q", code, stderr.String())
	}
	got := r.runCalls[0]
	if !slices.Contains(got, "--session-duration=43200") {
		t.Fatalf("run args=%v", got)
	}
}

func TestRun_SessionDurationFlagOverridesSaml2awsConfig(t *testing.T) {
	r := &fakeRunner{captureOut: "123456\n", runExit: 0}
	var stderr bytes.Buffer
	home := t.TempDir()
	config := "username = alice\naws_session_duration = 43200\n"
	if err := os.WriteFile(filepath.Join(home, ".saml2aws"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	code := runLogin(r, map[string]string{"HOME": home}, &stderr, "7200")
	if code != 0 {
		t.Fatalf("exit=%d want 0, stderr=%q", code, stderr.String())
	}
	got := r.runCalls[0]
	if !slices.Contains(got, "--session-duration=7200") || slices.Contains(got, "--session-duration=43200") {
		t.Fatalf("run args=%v", got)
	}
}

func TestRun_PropagatesSaml2awsExitCode(t *testing.T) {
	r := &fakeRunner{captureOut: "123456", runExit: 42}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr, "")
	if code != 42 {
		t.Fatalf("exit=%d want 42", code)
	}
}

func TestRun_OverrideTakesPrecedence(t *testing.T) {
	r := &fakeRunner{captureOut: "654321"}
	var stderr bytes.Buffer
	code := runLogin(r, map[string]string{
		"SAML2AWS_USERNAME":       "alice",
		"SAML2AWS_AUTO_TOTP_NAME": "Custom Entry",
	}, &stderr, "")
	if code != 0 {
		t.Fatalf("exit=%d want 0, stderr=%q", code, stderr.String())
	}
	if r.captureCalls[0][1] != "Custom Entry" {
		t.Fatalf("captureCalls=%v", r.captureCalls)
	}
}

func TestPrintZshInitRequiresUsername(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := printZshInit(&stdout, &stderr, map[string]string{"HOME": t.TempDir()})
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "saml2aws configure") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestPrintZshInitUsesSaml2awsConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".saml2aws"), []byte("username = alice@example.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := printZshInit(&stdout, &stderr, map[string]string{"HOME": home})
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "saml2aws-auto.plugin.zsh") {
		t.Fatalf("stdout=%q", got)
	}
	if !strings.Contains(got, `local saml2aws_auto_plugin`) || !strings.Contains(got, `unset saml2aws_auto_bin saml2aws_auto_plugin`) {
		t.Fatalf("stdout missing direct zsh setup snippet: %q", got)
	}
	if !strings.Contains(got, `${commands[saml2aws-auto]:A}`) {
		t.Fatalf("stdout should derive plugin path from command path: %q", got)
	}
	if strings.Contains(got, "zinit snippet") {
		t.Fatalf("stdout should not prefer zinit snippet: %q", got)
	}
}

func TestCheckSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	tests := []struct {
		name    string
		content string
		want    string
		minutes int
	}{
		{"missing token", "[default]\naws_access_key_id=x\n", statusUnknown, 0},
		{"invalid token", "x_security_token_expires = nope\n", statusUnknown, 0},
		{"expired", "x_security_token_expires = 2026-05-11T11:59:00+09:00\n", statusExpired, 0},
		{"expiring soon", "x_security_token_expires = 2026-05-11T12:30:00+09:00\n", statusExpiring, 30},
		{"valid", "x_security_token_expires = 2026-05-11T14:00:00+09:00\n", statusValid, 0},
		{"bsd date style", "x_security_token_expires = 2026-05-11T12:30:00+0900\n", statusExpiring, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			got := checkSession(path, now)
			if got.kind != tt.want || got.minutes != tt.minutes {
				t.Fatalf("got=%+v want kind=%s minutes=%d", got, tt.want, tt.minutes)
			}
		})
	}
}

func TestSessionStatusPromptText(t *testing.T) {
	tests := []struct {
		status sessionStatus
		want   string
	}{
		{sessionStatus{kind: statusUnknown}, "unknown"},
		{sessionStatus{kind: statusValid}, "valid"},
		{sessionStatus{kind: statusExpired}, "expired"},
		{sessionStatus{kind: statusExpiring, minutes: 12}, "expiring_soon:12"},
		{sessionStatus{kind: "broken"}, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.promptText(); got != tt.want {
			t.Fatalf("got=%q want=%q", got, tt.want)
		}
	}
}

func TestRunStatusPrintsPromptCompatibleStatus(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "x_security_token_expires = 2026-05-11T12:30:00+09:00\n"
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	if code := runStatus(map[string]string{"HOME": home}, &stdout, now); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "expiring_soon:30" {
		t.Fatalf("got=%q", got)
	}
}

func TestShouldPromptSuppressExpired(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "suppress")
	if err := writeSuppress(path, suppressRecord{typ: "expired", value: "2026-05-11"}); err != nil {
		t.Fatal(err)
	}
	if shouldPrompt(path, sessionStatus{kind: statusExpired}, now) {
		t.Fatal("same-day expired suppress should skip prompt")
	}
	if !shouldPrompt(path, sessionStatus{kind: statusExpired}, now.AddDate(0, 0, 1)) {
		t.Fatal("next-day expired suppress should prompt")
	}
}

func TestShouldPromptSuppressExpiringIncrementsCount(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "suppress")
	if err := writeSuppress(path, suppressRecord{typ: "expiring", value: "seed", count: 0}); err != nil {
		t.Fatal(err)
	}

	if shouldPrompt(path, sessionStatus{kind: statusExpiring, minutes: 30}, now) {
		t.Fatal("first expiring suppress check should skip prompt")
	}
	record, _ := readSuppress(path)
	if record.count != 1 {
		t.Fatalf("count=%d want 1", record.count)
	}

	if shouldPrompt(path, sessionStatus{kind: statusExpiring, minutes: 30}, now) {
		t.Fatal("second expiring suppress check should skip prompt")
	}
	record, _ = readSuppress(path)
	if record.count != 2 {
		t.Fatalf("count=%d want 2", record.count)
	}

	if !shouldPrompt(path, sessionStatus{kind: statusExpiring, minutes: 30}, now) {
		t.Fatal("third expiring suppress check should prompt")
	}
	if !shouldPrompt(path, sessionStatus{kind: statusExpired}, now) {
		t.Fatal("expired session should override expiring suppress")
	}
}

func TestRunCheckSkipsValidAndUnknown(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	for _, content := range []string{
		"x_security_token_expires = 2026-05-11T14:00:00Z\n",
		"x_security_token_expires = nope\n",
	} {
		home := t.TempDir()
		awsDir := filepath.Join(home, ".aws")
		if err := os.MkdirAll(awsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		r := &fakeRunner{captureOut: "123456\n"}
		var stdout, stderr bytes.Buffer
		code := runCheck(r, map[string]string{"HOME": home}, os.Stdin, &stdout, &stderr, now)
		if code != 0 {
			t.Fatalf("exit=%d want 0", code)
		}
		if len(r.captureCalls) != 0 || len(r.runCalls) != 0 {
			t.Fatalf("unexpected login calls: capture=%v run=%v", r.captureCalls, r.runCalls)
		}
	}
}
