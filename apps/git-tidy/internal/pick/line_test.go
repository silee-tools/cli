package pick

import (
	"reflect"
	"strings"
	"testing"
)

func TestRunLineTogglesAndConfirms(t *testing.T) {
	sel := NewSelection([]string{"a", "b", "c"})
	labels := []string{"(gone)", "(stale)", "(merged)"}
	// 2번 토글 해제 → 빈 줄 완료 → y 확정
	in := strings.NewReader("2\n\ny\n")
	got, ok := RunLine(sel, labels, in, &strings.Builder{})
	if !ok {
		t.Fatal("확정돼야 함")
	}
	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("got %v, want [a c]", got)
	}
}

func TestRunLineCancel(t *testing.T) {
	sel := NewSelection([]string{"a"})
	in := strings.NewReader("q\n")
	if _, ok := RunLine(sel, []string{""}, in, &strings.Builder{}); ok {
		t.Error("q 는 취소여야 함")
	}
}
