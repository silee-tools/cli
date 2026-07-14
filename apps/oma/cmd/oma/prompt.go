package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var errCancelled = errors.New("oma: 사용자가 취소했습니다")

type promptOption struct {
	Value string
	Label string
}

type Prompter interface {
	Select(label string, options []promptOption) (string, error)
	Confirm(message string) (bool, error)
}

type terminalPrompter struct {
	input  io.Reader
	output io.Writer
}

func (p terminalPrompter) Select(label string, options []promptOption) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("oma prep: %s 후보가 없습니다", label)
	}
	if _, err := fmt.Fprintln(p.output, label); err != nil {
		return "", err
	}
	for i, option := range options {
		if _, err := fmt.Fprintf(p.output, "  %d) %s\n", i+1, option.Label); err != nil {
			return "", err
		}
	}
	if _, err := fmt.Fprint(p.output, "> "); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(p.input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errCancelled
	}
	if index, err := strconv.Atoi(answer); err == nil && index > 0 && index <= len(options) {
		return options[index-1].Value, nil
	}
	for _, option := range options {
		if answer == option.Value {
			return option.Value, nil
		}
	}
	return "", fmt.Errorf("oma prep: 유효하지 않은 선택입니다: %s", answer)
}

func (p terminalPrompter) Confirm(message string) (bool, error) {
	if _, err := fmt.Fprintf(p.output, "%s [y/N] ", message); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(p.input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
