package shell

import (
	"strings"
	"testing"
)

func TestInitZshIncludesJgwFunction(t *testing.T) {
	out := InitZsh()
	if !strings.Contains(out, "jgw()") {
		t.Errorf("InitZsh output missing jgw(): %s", out)
	}
}

func TestInitBashIncludesJgwFunction(t *testing.T) {
	out := InitBash()
	if !strings.Contains(out, "jgw()") {
		t.Errorf("InitBash output missing jgw(): %s", out)
	}
}
