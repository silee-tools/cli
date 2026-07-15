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
	Input(label string) (string, error)
	Confirm(message string) (bool, error)
}

type terminalPrompter struct {
	input  io.Reader
	output io.Writer
	reader *bufio.Reader
}

func (p *terminalPrompter) Select(label string, options []promptOption) (string, error) {
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
	answer, err := p.readLine()
	if err != nil {
		return "", err
	}
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

func (p *terminalPrompter) Input(label string) (string, error) {
	if _, err := fmt.Fprintf(p.output, "%s ", label); err != nil {
		return "", err
	}
	answer, err := p.readLine()
	if err != nil {
		return "", err
	}
	if answer == "" {
		return "", errCancelled
	}
	return answer, nil
}

func (p *terminalPrompter) Confirm(message string) (bool, error) {
	if _, err := fmt.Fprintf(p.output, "%s [y/N] ", message); err != nil {
		return false, err
	}
	answer, err := p.readLine()
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(answer)
	return answer == "y" || answer == "yes", nil
}

func (p *terminalPrompter) readLine() (string, error) {
	if p.reader == nil {
		p.reader = bufio.NewReader(p.input)
	}
	line, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
