package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// IsInteractive returns true if stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

var stdinReader = bufio.NewReader(os.Stdin)

// SelectOption displays a numbered menu and returns the selected index.
// defaultIdx is the pre-selected option (0-based).
func SelectOption(prompt string, options []string, defaultIdx int) (int, error) {
	fmt.Println()
	Info("%s", prompt)
	fmt.Println()

	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = "→ "
		}
		fmt.Printf("  %s%d. %s\n", marker, i+1, opt)
	}

	fmt.Println()
	fmt.Printf("  Choice [%d]: ", defaultIdx+1)

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("input cancelled")
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return defaultIdx, nil
	}

	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		return -1, fmt.Errorf("invalid choice: %s", line)
	}

	return n - 1, nil
}

// Confirm displays a Y/n or y/N prompt and returns the user's choice.
// On EOF or unrecognized input, returns false (safe default).
func Confirm(prompt string, defaultYes bool) (bool, error) {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}

	fmt.Printf("\n  %s %s: ", prompt, hint)

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("input cancelled")
	}
	line = strings.TrimSpace(strings.ToLower(line))

	if line == "" {
		return defaultYes, nil
	}

	switch line {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		Warning("unrecognized input %q, treating as 'no'", line)
		return false, nil
	}
}

// ReadLine displays a prompt and reads a single line of input.
func ReadLine(prompt string) (string, error) {
	fmt.Printf("\n  %s", prompt)

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("input cancelled")
	}

	return strings.TrimSpace(line), nil
}
