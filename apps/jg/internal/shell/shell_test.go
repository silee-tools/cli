package shell

import (
	"strings"
	"testing"
)

func TestInitZshIncludesJgwFunction(t *testing.T) {
	out := InitZsh()
	checks := []string{
		"jgw()",
		"command jgw \"$@\"",
		`builtin cd "$result"`,
		"return $ret",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("InitZsh output missing %q", c)
		}
	}
}

func TestInitBashIncludesJgwFunction(t *testing.T) {
	out := InitBash()
	checks := []string{
		"jgw()",
		"command jgw \"$@\"",
		`builtin cd "$result"`,
		"return $ret",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("InitBash output missing %q", c)
		}
	}
}
