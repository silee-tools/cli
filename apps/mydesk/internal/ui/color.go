package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

var noColor bool

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	dim     = "\033[2m"
	bold    = "\033[1m"
)

func Init(disableColor bool) {
	noColor = disableColor || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd()))
}

func colorize(color, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	if noColor {
		return msg
	}
	return color + msg + reset
}

func Success(format string, a ...any) {
	fmt.Println(colorize(green, " ✓ "+format, a...))
}

func Warning(format string, a ...any) {
	fmt.Println(colorize(yellow, " ! "+format, a...))
}

func Error(format string, a ...any) {
	fmt.Fprintln(os.Stderr, colorize(red, " ✗ "+format, a...))
}

func Info(format string, a ...any) {
	fmt.Println(colorize(blue, " ℹ "+format, a...))
}

func DryRun(format string, a ...any) {
	fmt.Println(colorize(dim, " ⊘ [dry-run] "+format, a...))
}

func Bold(s string) string {
	if noColor {
		return s
	}
	return bold + s + reset
}

func Dim(s string) string {
	if noColor {
		return s
	}
	return dim + s + reset
}
