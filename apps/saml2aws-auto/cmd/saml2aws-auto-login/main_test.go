package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
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

func TestRun_MissingSaml2aws(t *testing.T) {
	r := &fakeRunner{missing: map[string]bool{"saml2aws": true}}
	var stderr bytes.Buffer
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
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
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
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
	code := run(r, map[string]string{}, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	if !strings.Contains(stderr.String(), "SAML2AWS_USERNAME unset") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRun_TOTPFailure(t *testing.T) {
	r := &fakeRunner{captureErr: errors.New("no entry")}
	var stderr bytes.Buffer
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
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
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
}

func TestRun_HappyPath(t *testing.T) {
	r := &fakeRunner{captureOut: "123456\n", runExit: 0}
	var stderr bytes.Buffer
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
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

func TestRun_PropagatesSaml2awsExitCode(t *testing.T) {
	r := &fakeRunner{captureOut: "123456", runExit: 42}
	var stderr bytes.Buffer
	code := run(r, map[string]string{"SAML2AWS_USERNAME": "alice"}, &stderr)
	if code != 42 {
		t.Fatalf("exit=%d want 42", code)
	}
}

func TestRun_OverrideTakesPrecedence(t *testing.T) {
	r := &fakeRunner{captureOut: "654321"}
	var stderr bytes.Buffer
	code := run(r, map[string]string{
		"SAML2AWS_USERNAME":       "alice",
		"SAML2AWS_AUTO_TOTP_NAME": "Custom Entry",
	}, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d want 0, stderr=%q", code, stderr.String())
	}
	if r.captureCalls[0][1] != "Custom Entry" {
		t.Fatalf("captureCalls=%v", r.captureCalls)
	}
}
