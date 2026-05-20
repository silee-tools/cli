package shell

import (
	"strings"
	"testing"
)

// 셸 통합이 zsh/bash 양쪽에서 jg + jgw 함수와 각 셸 고유 hook 을 모두 emit 하는지를
// 잠근다. plugin/jg.plugin.zsh / plugin/jg.plugin.bash 가 단일 진실 소스이므로
// 한쪽만 갱신하고 다른 쪽을 빠뜨리는 drift 를 잡아낸다.

func TestInitZshContainsAllRequiredTokens(t *testing.T) {
	out := InitZsh()
	required := []string{
		"jg()",
		"jgw()",
		"command jg \"$@\"",
		"command jgw \"$@\"",
		`builtin cd "$result"`,
		"return $ret",
		"_jg_chpwd",
		"add-zsh-hook chpwd _jg_chpwd",
	}
	for _, tok := range required {
		if !strings.Contains(out, tok) {
			t.Errorf("InitZsh output missing token %q", tok)
		}
	}
}

func TestInitBashContainsAllRequiredTokens(t *testing.T) {
	out := InitBash()
	required := []string{
		"jg()",
		"jgw()",
		"command jg \"$@\"",
		"command jgw \"$@\"",
		`builtin cd "$result"`,
		"return $ret",
		"_jg_prompt_command",
		"PROMPT_COMMAND",
	}
	for _, tok := range required {
		if !strings.Contains(out, tok) {
			t.Errorf("InitBash output missing token %q", tok)
		}
	}
}

func TestInitRejectsUnsupportedShell(t *testing.T) {
	_, err := Init("fish")
	if err == nil {
		t.Error("Init(fish) expected error, got nil")
	}
}
