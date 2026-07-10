package runtimechannel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runtimePaths(t *testing.T, tool string) (prefix, statePath, executable string) {
	t.Helper()
	prefix = t.TempDir()
	statePath = filepath.Join(prefix, "var", "silee-tools", tool, "active-channel")
	executable = filepath.Join(prefix, "opt", tool, "bin", tool)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	return prefix, statePath, executable
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseExecutableSelectsFixedReleaseForDevBuild(t *testing.T) {
	_, statePath, executable := runtimePaths(t, "totp")
	writeFile(t, executable, "release", 0o755)
	writeFile(t, statePath, "channel=release\n", 0o644)

	got, err := ReleaseExecutable("dev", statePath, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if got != executable {
		t.Fatalf("ReleaseExecutable() = %q, want %q", got, executable)
	}
}

func TestReleaseExecutableRejectsUnknownChannel(t *testing.T) {
	_, statePath, _ := runtimePaths(t, "totp")
	writeFile(t, statePath, "channel=unexpected\n", 0o644)

	_, err := ReleaseExecutable("dev", statePath, "totp")
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("ReleaseExecutable() error = %v, want invalid state error", err)
	}
}

func TestReleaseExecutableKeepsDevWhenStateDoesNotExist(t *testing.T) {
	_, statePath, _ := runtimePaths(t, "totp")

	got, err := ReleaseExecutable("dev", statePath, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ReleaseExecutable() = %q, want local dev execution", got)
	}
}

func TestReleaseExecutableKeepsDevForDevChannel(t *testing.T) {
	_, statePath, _ := runtimePaths(t, "totp")
	writeFile(t, statePath, "channel=dev\n", 0o644)

	got, err := ReleaseExecutable("dev", statePath, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ReleaseExecutable() = %q, want local dev execution", got)
	}
}

func TestReleaseExecutableIgnoresChannelForReleaseBuild(t *testing.T) {
	_, statePath, _ := runtimePaths(t, "totp")
	writeFile(t, statePath, "channel=unexpected\n", 0o644)

	got, err := ReleaseExecutable("1.2.3", statePath, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ReleaseExecutable() = %q, want release binary to stay local", got)
	}
}

func TestReleaseExecutableRejectsMissingReleaseBinary(t *testing.T) {
	_, statePath, _ := runtimePaths(t, "totp")
	writeFile(t, statePath, "channel=release\n", 0o644)

	_, err := ReleaseExecutable("dev", statePath, "totp")
	if err == nil || !strings.Contains(err.Error(), "release executable") {
		t.Fatalf("ReleaseExecutable() error = %v, want missing release executable error", err)
	}
}

func TestReleaseExecutableFindsStateAfterHomebrewIsInstalled(t *testing.T) {
	prefix, statePath, executable := runtimePaths(t, "totp")
	writeFile(t, executable, "release", 0o755)
	writeFile(t, statePath, "channel=release\n", 0o644)

	original := findHomebrewPrefix
	findHomebrewPrefix = func() (string, error) { return prefix, nil }
	t.Cleanup(func() { findHomebrewPrefix = original })

	got, err := ReleaseExecutable("dev", "", "totp")
	if err != nil {
		t.Fatal(err)
	}
	if got != executable {
		t.Fatalf("ReleaseExecutable() = %q, want %q", got, executable)
	}
}

func TestReleaseExecutableRejectsExecutableOverride(t *testing.T) {
	_, statePath, executable := runtimePaths(t, "totp")
	override := filepath.Join(t.TempDir(), "override")
	writeFile(t, override, "override", 0o755)
	writeFile(t, executable, "release", 0o755)
	writeFile(t, statePath, "channel=release\nexecutable="+override+"\n", 0o644)

	_, err := ReleaseExecutable("dev", statePath, "totp")
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("ReleaseExecutable() error = %v, want invalid state error", err)
	}
}

func TestReleaseExecutableRejectsUnexpectedStatePath(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "active-channel")
	writeFile(t, statePath, "channel=release\n", 0o644)

	_, err := ReleaseExecutable("dev", statePath, "totp")
	if err == nil || !strings.Contains(err.Error(), "unexpected state path") {
		t.Fatalf("ReleaseExecutable() error = %v, want unexpected state path error", err)
	}
}
