package reason

import (
	"strings"
	"testing"
)

func TestDescription(t *testing.T) {
	cases := map[string]string{
		"gone":     "upstream 추적 브랜치",
		"merged":   "그대로 들어간",
		"absorbed": "같은 Jira 티켓",
		"stale":    "stale 기준일",
	}
	for signal, want := range cases {
		got := Description(signal)
		if !strings.Contains(got, want) {
			t.Fatalf("Description(%q) = %q, want containing %q", signal, got, want)
		}
	}
}

func TestDescriptionUnknownSignal(t *testing.T) {
	if got := Description("unknown"); got != "" {
		t.Fatalf("unknown signal description = %q, want empty", got)
	}
}
