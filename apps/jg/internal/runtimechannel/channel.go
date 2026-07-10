package runtimechannel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var findHomebrewPrefix = func() (string, error) {
	out, err := exec.Command("brew", "--prefix").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func ReleaseExecutable(version, statePath, tool string) (string, error) {
	if version != "dev" {
		return "", nil
	}
	var prefix string
	if statePath == "" {
		var err error
		prefix, err = findHomebrewPrefix()
		if err != nil || prefix == "" {
			return "", nil
		}
		statePath = filepath.Join(prefix, "var", "silee-tools", tool, "active-channel")
	} else {
		var err error
		prefix, err = prefixFromStatePath(statePath, tool)
		if err != nil {
			return "", err
		}
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	switch strings.TrimSpace(string(data)) {
	case "channel=dev":
		return "", nil
	case "channel=release":
		executable := filepath.Join(prefix, "opt", tool, "bin", tool)
		info, err := os.Stat(executable)
		if err != nil {
			return "", fmt.Errorf("runtime channel: release executable %q: %w", executable, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("runtime channel: release executable %q is not executable", executable)
		}
		return executable, nil
	default:
		return "", fmt.Errorf("runtime channel: invalid state in %q", statePath)
	}
}

func prefixFromStatePath(statePath, tool string) (string, error) {
	if tool == "" || filepath.Base(tool) != tool {
		return "", fmt.Errorf("runtime channel: invalid tool name %q", tool)
	}
	clean := filepath.Clean(statePath)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("runtime channel: state path must be absolute: %q", statePath)
	}
	prefix := clean
	for range 4 {
		prefix = filepath.Dir(prefix)
	}
	expected := filepath.Join(prefix, "var", "silee-tools", tool, "active-channel")
	if clean != expected {
		return "", fmt.Errorf("runtime channel: unexpected state path %q", statePath)
	}
	return prefix, nil
}
