package prep

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSlugKeepsKorean(t *testing.T) {
	got, err := Slug(" 결제 완료 ✅ / 영수증 ")
	if err != nil || got != "결제-완료-영수증" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestSlugNormalizesAndLimitsUnicodeCharacters(t *testing.T) {
	decomposed := "Cafe\u0301 " + strings.Repeat("한", 60)
	got, err := Slug(decomposed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Café-") {
		t.Fatalf("slug = %q, want NFC prefix", got)
	}
	if count := utf8.RuneCountInString(got); count != 50 {
		t.Fatalf("rune count = %d, want 50", count)
	}
}

func TestSlugFlattensSeparatorsAndRejectsEmpty(t *testing.T) {
	got, err := Slug(" alpha___beta...gamma///delta ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "alpha-beta-gamma-delta" {
		t.Fatalf("slug = %q", got)
	}
	if _, err := Slug("✅ ~~ ^^ :: ?? ** [[ "); err == nil {
		t.Fatal("Slug() error = nil, want empty slug error")
	}
}

func TestNormalizeIssueKey(t *testing.T) {
	got, err := NormalizeIssueKey(" abc-123 ")
	if err != nil || got != "ABC-123" {
		t.Fatalf("got %q, err %v", got, err)
	}
	for _, value := range []string{"", "ABC", "ABC-0", "-123", "한글-123"} {
		if _, err := NormalizeIssueKey(value); err == nil {
			t.Errorf("NormalizeIssueKey(%q) error = nil", value)
		}
	}
}

func TestNamesByInputKind(t *testing.T) {
	today := time.Date(2026, 7, 14, 9, 0, 0, 0, time.Local)
	tests := []struct {
		name         string
		kind         InputKind
		key          string
		title        string
		wantBranch   string
		wantWorktree string
	}{
		{name: "jira", kind: InputJira, key: "abc-123", title: "제목", wantBranch: "feature/ABC-123-제목", wantWorktree: "abc-123-제목"},
		{name: "description", kind: InputDescription, title: "설명", wantBranch: "feature/설명", wantWorktree: "설명"},
		{name: "empty", kind: InputEmpty, wantBranch: "feature/temp-2026-07-14", wantWorktree: "temp-2026-07-14"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildNames(tc.kind, "feature", tc.key, tc.title, today)
			if err != nil {
				t.Fatal(err)
			}
			if got.Branch != tc.wantBranch || got.Worktree != tc.wantWorktree {
				t.Fatalf("names = %+v, want branch %q worktree %q", got, tc.wantBranch, tc.wantWorktree)
			}
		})
	}
}

func TestBranchNameRejectsInvalidType(t *testing.T) {
	if _, err := BranchName(InputDescription, "Feature", "", "설명", time.Now()); err == nil {
		t.Fatal("BranchName() error = nil")
	}
}
