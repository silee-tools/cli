package shell

import (
	"fmt"

	"github.com/silee-tools/jg/plugin"
)

func InitZsh() string {
	return plugin.Zsh
}

func InitBash() string {
	return plugin.Bash
}

func Init(shellName string) (string, error) {
	switch shellName {
	case "zsh":
		return InitZsh(), nil
	case "bash":
		return InitBash(), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s (supported: zsh, bash)", shellName)
	}
}
