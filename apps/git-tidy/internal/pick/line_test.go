package pick

import (
	"reflect"
	"strings"
	"testing"
)

func lineItems() []Item {
	return []Item{
		{Name: "g1", Signal: "gone", Checked: true},
		{Name: "m1", Signal: "merged", Checked: false},
		{Name: "a1", Signal: "absorbed", AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: absorbed base", Checked: false},
		{Name: "s1", Signal: "stale", AgeDays: 34, WorktreePath: "/tmp/wt/s1", Checked: false},
	}
}

func TestRunLineTogglesAndConfirms(t *testing.T) {
	sel := NewSelection(lineItems())
	// 2번(m1) 토글 체크 → 빈 줄 완료 → y 확정. g1 은 기본 체크.
	in := strings.NewReader("2\n\ny\n")
	got, ok := RunLine(sel, in, &strings.Builder{})
	if !ok {
		t.Fatal("확정돼야 함")
	}
	if !reflect.DeepEqual(got, []string{"g1", "m1"}) {
		t.Errorf("got %v, want [g1 m1]", got)
	}
}

func TestRunLineCancel(t *testing.T) {
	sel := NewSelection(lineItems())
	in := strings.NewReader("q\n")
	if _, ok := RunLine(sel, in, &strings.Builder{}); ok {
		t.Error("q 는 취소여야 함")
	}
}

func TestRunLineRendersGroupsAndMeta(t *testing.T) {
	sel := NewSelection(lineItems())
	var out strings.Builder
	in := strings.NewReader("q\n")
	RunLine(sel, in, &out)
	s := out.String()
	for _, want := range []string{
		"gone (1)",
		"merged (1)",
		"absorbed (1)",
		"stale (1)",
		"같은 Jira 티켓의 더 최신 base 커밋",
		"base: 9a640b52f [ABC-1375] feat: absorbed base",
		"34일 경과",
		"⌂ s1",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("출력에 %q 가 없음:\n%s", want, s)
		}
	}
}
