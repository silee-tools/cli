package scheduler

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

const label = "com.silee-tools.jg.clean"

type InstallResult struct {
	PlistPath string
}

type StatusResult struct {
	Installed bool
	PlistPath string
}

func Install(jgPath string) (InstallResult, error) {
	if err := ensureSupported(runtime.GOOS); err != nil {
		return InstallResult{}, err
	}

	paths, err := resolvePaths()
	if err != nil {
		return InstallResult{}, err
	}
	jgPath, err = resolveExecutable(jgPath)
	if err != nil {
		return InstallResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(paths.plistPath), 0755); err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(paths.stateDir, 0755); err != nil {
		return InstallResult{}, err
	}

	content := BuildPlist(Config{
		JGPath:  jgPath,
		OutPath: filepath.Join(paths.stateDir, "clean.log"),
		ErrPath: filepath.Join(paths.stateDir, "clean.err.log"),
		Hour:    9,
		Minute:  0,
	})
	if err := os.WriteFile(paths.plistPath, []byte(content), 0644); err != nil {
		return InstallResult{}, err
	}

	_ = launchctl("bootout", launchTarget(), paths.plistPath)
	if err := launchctl("bootstrap", launchTarget(), paths.plistPath); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{PlistPath: paths.plistPath}, nil
}

func Remove() (bool, error) {
	if err := ensureSupported(runtime.GOOS); err != nil {
		return false, err
	}

	paths, err := resolvePaths()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(paths.plistPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	_ = launchctl("bootout", launchTarget(), paths.plistPath)
	if err := os.Remove(paths.plistPath); err != nil {
		return false, err
	}
	return true, nil
}

func Status() (StatusResult, error) {
	if err := ensureSupported(runtime.GOOS); err != nil {
		return StatusResult{}, err
	}

	paths, err := resolvePaths()
	if err != nil {
		return StatusResult{}, err
	}
	_, err = os.Stat(paths.plistPath)
	if err == nil {
		return StatusResult{Installed: true, PlistPath: paths.plistPath}, nil
	}
	if os.IsNotExist(err) {
		return StatusResult{Installed: false, PlistPath: paths.plistPath}, nil
	}
	return StatusResult{}, err
}

type Config struct {
	JGPath  string
	OutPath string
	ErrPath string
	Hour    int
	Minute  int
}

func BuildPlist(config Config) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>clean</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>%d</integer>
    <key>Minute</key>
    <integer>%d</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, label, escape(config.JGPath), config.Hour, config.Minute, escape(config.OutPath), escape(config.ErrPath))
}

func ensureSupported(goos string) error {
	if goos != "darwin" {
		return fmt.Errorf("jg scheduler is only supported on macOS launchd")
	}
	return nil
}

type paths struct {
	plistPath string
	stateDir  string
}

func resolvePaths() (paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return paths{}, err
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}

	return paths{
		plistPath: filepath.Join(home, "Library", "LaunchAgents", label+".plist"),
		stateDir:  filepath.Join(stateHome, "jg"),
	}, nil
}

func resolveExecutable(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func launchTarget() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

func launchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %v failed: %w: %s", args, err, string(out))
	}
	return nil
}

func escape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}
