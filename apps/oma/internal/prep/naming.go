package prep

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	issueKeyPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[1-9][0-9]*$`)
	branchTypePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

func NormalizeIssueKey(value string) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(value))
	if !issueKeyPattern.MatchString(key) {
		return "", fmt.Errorf("oma prep: 유효하지 않은 Jira 키입니다: %q", value)
	}
	return key, nil
}

func Slug(value string) (string, error) {
	var result []rune
	separator := false
	for _, r := range norm.NFC.String(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if separator && len(result) > 0 {
				result = append(result, '-')
			}
			result = append(result, r)
			separator = false
		case r == '~' || r == '^' || r == ':' || r == '?' || r == '*' || r == '[' || r == '\\' || unicode.IsControl(r):
			continue
		case unicode.IsSpace(r) || unicode.IsPunct(r):
			separator = true
		}
		if len(result) >= 50 {
			result = result[:50]
			break
		}
	}
	slug := strings.Trim(string(result), "-")
	if slug == "" {
		return "", fmt.Errorf("oma prep: 정규화한 제목이 비어 있습니다")
	}
	return slug, nil
}

func BranchName(kind InputKind, branchType, key, title string, today time.Time) (string, error) {
	if !branchTypePattern.MatchString(branchType) {
		return "", fmt.Errorf("oma prep: 유효하지 않은 브랜치 type입니다: %q", branchType)
	}
	component, err := nameComponent(kind, key, title, today, false)
	if err != nil {
		return "", err
	}
	return branchType + "/" + component, nil
}

func WorktreeName(kind InputKind, key, title string, today time.Time) (string, error) {
	return nameComponent(kind, key, title, today, true)
}

func BuildNames(kind InputKind, branchType, key, title string, today time.Time) (Names, error) {
	branch, err := BranchName(kind, branchType, key, title, today)
	if err != nil {
		return Names{}, err
	}
	worktree, err := WorktreeName(kind, key, title, today)
	if err != nil {
		return Names{}, err
	}
	return Names{Branch: branch, Worktree: worktree}, nil
}

func nameComponent(kind InputKind, key, title string, today time.Time, lowerKey bool) (string, error) {
	switch kind {
	case InputJira:
		normalizedKey, err := NormalizeIssueKey(key)
		if err != nil {
			return "", err
		}
		if lowerKey {
			normalizedKey = strings.ToLower(normalizedKey)
		}
		slug, err := Slug(title)
		if err != nil {
			return "", err
		}
		return normalizedKey + "-" + slug, nil
	case InputDescription:
		return Slug(title)
	case InputEmpty:
		if today.IsZero() {
			return "", fmt.Errorf("oma prep: 빈 작업에는 날짜가 필요합니다")
		}
		return "temp-" + today.Format("2006-01-02"), nil
	default:
		return "", fmt.Errorf("oma prep: 알 수 없는 작업 입력 종류입니다: %q", kind)
	}
}
