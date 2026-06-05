# git-tidy Absorbed Branches Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git-tidy 가 스쿼시 머지되었거나 더 큰 작업에 흡수된 것으로 보이는 로컬 브랜치를 `absorbed` 후보로 보여주게 만든다.

**Architecture:** `classify` 는 새 `absorbed` 신호를 순수 함수로 판정하고, `gitx` 는 base 브랜치 커밋 목록만 구조화해서 넘긴다. dry-run, 체크박스 TUI, 줄 기반 선택은 신호별 설명 문구를 한 곳에서 가져와 같은 화면 언어를 사용한다.

**Tech Stack:** Go 1.23, git CLI, bubbletea, lipgloss, 기존 `apps/git-tidy` 단위 테스트.

---

## 실행 기준

모든 명령은 이 저장소의 루트에서 실행한다고 가정한다. `go` 가 mise trust 문제로 실행되지
않으면, 먼저 이 worktree 의 `.mise.toml` 신뢰 상태를 정리하거나 Go 1.23 바이너리를
`PATH` 앞에 두고 같은 명령을 실행한다.

## 파일 구조

- Modify: `apps/git-tidy/internal/classify/classify.go`
  - `absorbed` 신호, Jira 티켓 ID 추출, base 커밋 근거 선택, 신호별 설명 문구를 담당한다.
- Modify: `apps/git-tidy/internal/classify/classify_test.go`
  - `absorbed` 판정과 신호 순서를 순수 함수 테스트로 고정한다.
- Modify: `apps/git-tidy/internal/gitx/gitx.go`
  - base 브랜치의 커밋 해시, 제목, committer date 를 읽어 구조화한다.
- Modify: `apps/git-tidy/internal/gitx/gitx_test.go`
  - base 커밋 파서가 NUL 구분 형식을 정확히 읽는지 검증한다.
- Create: `apps/git-tidy/internal/reason/reason.go`
  - 신호별 설명 문구를 한 곳에서 제공한다.
- Test: `apps/git-tidy/internal/reason/reason_test.go`
  - 신호별 설명 문구가 비어 있지 않고 핵심 표현을 담는지 확인한다.
- Modify: `apps/git-tidy/cmd/git-tidy/main.go`
  - base 커밋 목록을 분류 입력에 넣고, `absorbed` 기본 체크를 해제하며, dry-run 에 설명과 근거 커밋을 출력한다.
- Modify: `apps/git-tidy/internal/pick/model.go`
  - 선택 항목에 `absorbed` 근거 커밋 정보를 보존한다.
- Modify: `apps/git-tidy/internal/pick/model_test.go`
  - `absorbed` 항목이 기본 체크되지 않고 근거 필드를 보존하는지 확인한다.
- Modify: `apps/git-tidy/internal/pick/line.go`
  - 줄 기반 선택 화면에 그룹 설명과 `absorbed` 근거 커밋을 표시한다.
- Modify: `apps/git-tidy/internal/pick/line_test.go`
  - 줄 기반 선택 화면의 설명 문구와 `absorbed` 근거 표시를 검증한다.
- Modify: `apps/git-tidy/internal/pick/tui.go`
  - TUI 헤더 아래 설명을 흐리게 보여주고, `absorbed` 색과 근거 표시를 추가한다.
- Modify: `apps/git-tidy/internal/pick/tui_test.go`
  - TUI row 구성과 설명 렌더링이 새 신호를 포함하는지 확인한다.

## Task 1: classify 에 absorbed 신호 추가

**Files:**
- Modify: `apps/git-tidy/internal/classify/classify.go`
- Test: `apps/git-tidy/internal/classify/classify_test.go`

- [ ] **Step 1: 실패하는 absorbed 판정 테스트를 추가한다**

`apps/git-tidy/internal/classify/classify_test.go` 끝에 다음 테스트를 추가한다.

```go
func TestClassifyAbsorbedWhenNewerBaseCommitHasSameTicket(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 10*86400
	baseTime := now - 2*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/example-absorbed-branch", CommitUnix: branchTime, Subject: "[ABC-1375] docs: 새 git worktree 셋업 안내 추가"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서", CommitUnix: baseTime},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	want := []Result{
		{
			Name:                 "claude/example-absorbed-branch",
			Signal:               SignalAbsorbed,
			AbsorbedByShortHash:  "9a640b52f",
			AbsorbedBySubject:    "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서",
			AbsorbedByCommitUnix: baseTime,
		},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("absorbed 후보 mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}
```

- [ ] **Step 2: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/classify
```

Expected: `Subject`, `BaseCommits`, `gitx.CommitRef`, `SignalAbsorbed`, `AbsorbedByShortHash` 같은 이름이 아직 없어서 컴파일 실패한다.

- [ ] **Step 3: classify 데이터 모델을 확장한다**

`apps/git-tidy/internal/classify/classify.go` 에 다음 필드를 추가한다.

```go
const (
	SignalGone     Signal = "gone"
	SignalMerged   Signal = "merged"
	SignalAbsorbed Signal = "absorbed"
	SignalStale    Signal = "stale"
)

type Input struct {
	Now           int64
	StaleDays     int
	Base          string
	Current       string
	Branches      []gitx.BranchRef
	Merged        map[string]bool
	Worktrees     map[string]string
	BaseCommits   []gitx.CommitRef
	MergeBaseUnix func(branch string) (int64, bool)
}

type Result struct {
	Name                 string
	Signal               Signal
	WorktreePath         string
	AgeDays              int
	AbsorbedByShortHash  string
	AbsorbedBySubject    string
	AbsorbedByCommitUnix int64
}
```

- [ ] **Step 4: Jira 티켓 추출과 absorbed 근거 선택 함수를 추가한다**

`apps/git-tidy/internal/classify/classify.go` 에 `regexp` import 를 추가하고, 파일 하단에 다음 함수를 넣는다.

```go
var ticketIDPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-[0-9]+\b`)

func ticketID(subject string) string {
	return ticketIDPattern.FindString(subject)
}

func absorbedBaseCommit(b gitx.BranchRef, in Input) (gitx.CommitRef, bool) {
	if in.Worktrees[b.Name] != "" {
		return gitx.CommitRef{}, false
	}
	id := ticketID(b.Subject)
	if id == "" {
		return gitx.CommitRef{}, false
	}
	var best gitx.CommitRef
	for _, c := range in.BaseCommits {
		if c.CommitUnix <= b.CommitUnix {
			continue
		}
		if ticketID(c.Subject) != id {
			continue
		}
		if best.CommitUnix == 0 || c.CommitUnix > best.CommitUnix {
			best = c
		}
	}
	return best, best.CommitUnix != 0
}
```

- [ ] **Step 5: candidateSignal 과 Classify 결과 생성을 수정한다**

`candidateSignal` 은 `gone`, `merged`, `absorbed`, `stale` 순서로 판정한다.

```go
func candidateSignal(b gitx.BranchRef, in Input, cutoff int64) (Signal, bool) {
	if b.UpstreamGone {
		return SignalGone, true
	}
	if in.Merged[b.Name] {
		return SignalMerged, true
	}
	if _, ok := absorbedBaseCommit(b, in); ok {
		return SignalAbsorbed, true
	}
	if isStale(b, in, cutoff) {
		return SignalStale, true
	}
	return "", false
}
```

`Classify` 의 `Result` 생성 직전에 absorbed 근거를 읽고 결과에 넣는다.

```go
		absorbedBy, _ := absorbedBaseCommit(b, in)
		out.ToDelete = append(out.ToDelete, Result{
			Name:                 b.Name,
			Signal:               sig,
			WorktreePath:         in.Worktrees[b.Name],
			AgeDays:              ageDaysFor(b, in, sig),
			AbsorbedByShortHash:  absorbedBy.ShortHash,
			AbsorbedBySubject:    absorbedBy.Subject,
			AbsorbedByCommitUnix: absorbedBy.CommitUnix,
		})
```

- [ ] **Step 6: 신호 정렬 순서를 수정한다**

`signalRank` 에 `absorbed` 를 `merged` 와 `stale` 사이에 넣는다.

```go
func signalRank(s Signal) int {
	switch s {
	case SignalGone:
		return 0
	case SignalMerged:
		return 1
	case SignalAbsorbed:
		return 2
	case SignalStale:
		return 3
	default:
		return 4
	}
}
```

- [ ] **Step 7: Green 을 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/classify
```

Expected: `ok github.com/silee-tools/git-tidy/internal/classify`.

- [ ] **Step 8: 커밋한다**

```bash
git add apps/git-tidy/internal/classify/classify.go apps/git-tidy/internal/classify/classify_test.go
git commit -m "feat(git-tidy): classify absorbed branches"
```

## Task 2: absorbed 예외 조건과 순서 테스트 보강

**Files:**
- Modify: `apps/git-tidy/internal/classify/classify_test.go`

- [ ] **Step 1: worktree 에 물린 브랜치 제외 테스트를 추가한다**

```go
func TestClassifyDoesNotAbsorbCheckedOutWorktreeBranch(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 10*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/in-progress", CommitUnix: branchTime, Subject: "[ABC-1375] docs: worktree 안내"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{"claude/in-progress": "/tmp/worktree"},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: 새 worktree 셋업", CommitUnix: now - 2*86400},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	if len(got.ToDelete) != 0 {
		t.Fatalf("worktree 에 체크아웃된 브랜치는 absorbed 후보가 아니어야 함: %+v", got.ToDelete)
	}
	if got.OtherCount != 1 {
		t.Fatalf("worktree 브랜치는 일반 브랜치로 남아야 함, OtherCount=%d", got.OtherCount)
	}
}
```

- [ ] **Step 2: 오래된 base 커밋 제외 테스트를 추가한다**

```go
func TestClassifyDoesNotAbsorbWhenBaseCommitIsOlder(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 2*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/newer-branch", CommitUnix: branchTime, Subject: "[ABC-1375] docs: 최신 브랜치 작업"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "111111111", Subject: "[ABC-1375] docs: 오래된 base 작업", CommitUnix: now - 10*86400},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	if len(got.ToDelete) != 0 {
		t.Fatalf("base 관련 커밋이 더 오래되면 absorbed 후보가 아니어야 함: %+v", got.ToDelete)
	}
	if got.OtherCount != 1 {
		t.Fatalf("브랜치는 일반 브랜치로 남아야 함, OtherCount=%d", got.OtherCount)
	}
}
```

- [ ] **Step 3: 신호 순서 테스트를 absorbed 포함으로 수정한다**

`TestClassifySortsBySignalThenName` 의 입력에 absorbed 브랜치 하나를 추가하고 기대값을 `gone`, `absorbed`, `stale` 순서로 바꾼다.

```go
Branches: []gitx.BranchRef{
	{Name: "zzz-stale", HasUpstream: true, CommitUnix: old},
	{Name: "aaa-stale", HasUpstream: true, CommitUnix: old},
	{Name: "mmm-gone", UpstreamGone: true, HasUpstream: true, CommitUnix: now},
	{Name: "bbb-absorbed", HasUpstream: true, CommitUnix: now - 10*day, Subject: "[ABC-1375] docs: absorbed branch"},
},
BaseCommits: []gitx.CommitRef{
	{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: absorbed base", CommitUnix: now - day},
},
```

Expected result:

```go
want := []Result{
	{Name: "mmm-gone", Signal: SignalGone},
	{
		Name:                 "bbb-absorbed",
		Signal:               SignalAbsorbed,
		AbsorbedByShortHash:  "9a640b52f",
		AbsorbedBySubject:    "[ABC-1375] feat: absorbed base",
		AbsorbedByCommitUnix: now - day,
	},
	{Name: "aaa-stale", Signal: SignalStale, AgeDays: 40},
	{Name: "zzz-stale", Signal: SignalStale, AgeDays: 40},
}
```

- [ ] **Step 4: classify 테스트 전체를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/classify
```

Expected: `ok github.com/silee-tools/git-tidy/internal/classify`.

- [ ] **Step 5: 커밋한다**

```bash
git add apps/git-tidy/internal/classify/classify_test.go
git commit -m "test(git-tidy): cover absorbed branch safeguards"
```

## Task 3: gitx 에 base 커밋 수집 추가

**Files:**
- Modify: `apps/git-tidy/internal/gitx/gitx.go`
- Test: `apps/git-tidy/internal/gitx/gitx_test.go`

- [ ] **Step 1: 실패하는 파서 테스트를 추가한다**

`apps/git-tidy/internal/gitx/gitx_test.go` 끝에 다음 테스트를 추가한다.

```go
func TestParseCommitLines(t *testing.T) {
	input := "9a640b52f\x001780376000\x00[ABC-1375] feat: 새 worktree 셋업\n" +
		"4d4f1d52f\x001780635662\x00[ABC-1399] fix(ci): landing-lighthouse\n"
	got := parseCommitLines(input)
	want := []CommitRef{
		{ShortHash: "9a640b52f", CommitUnix: 1780376000, Subject: "[ABC-1375] feat: 새 worktree 셋업"},
		{ShortHash: "4d4f1d52f", CommitUnix: 1780635662, Subject: "[ABC-1399] fix(ci): landing-lighthouse"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseCommitLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}
```

- [ ] **Step 2: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/gitx
```

Expected: `CommitRef` 와 `parseCommitLines` 가 없어서 컴파일 실패한다.

- [ ] **Step 3: CommitRef 와 파서를 추가한다**

`apps/git-tidy/internal/gitx/gitx.go` 에 다음 타입과 함수를 추가한다.

```go
// CommitRef 는 base 브랜치 커밋 하나의 absorbed 판정용 메타데이터다.
type CommitRef struct {
	ShortHash  string
	Subject    string
	CommitUnix int64
}

func parseCommitLines(out string) []CommitRef {
	var refs []CommitRef
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 3 {
			continue
		}
		unix, _ := strconv.ParseInt(f[1], 10, 64)
		refs = append(refs, CommitRef{
			ShortHash:  f[0],
			CommitUnix: unix,
			Subject:    f[2],
		})
	}
	return refs
}
```

- [ ] **Step 4: BranchRef 에 Subject 를 추가하고 파서를 수정한다**

`BranchRef` 와 `LocalBranches` 를 다음 형식으로 바꾼다.

```go
type BranchRef struct {
	Name         string
	HasUpstream  bool
	UpstreamGone bool
	CommitUnix   int64
	Subject      string
}

func LocalBranches() ([]BranchRef, error) {
	out, err := run("for-each-ref",
		"--format=%(refname:short)%00%(upstream:track)%00%(committerdate:unix)%00%(subject)",
		"refs/heads")
	if err != nil {
		return nil, err
	}
	return parseBranchLines(out), nil
}
```

`parseBranchLines` 는 4개 필드를 읽도록 바꾼다.

```go
		f := strings.Split(line, "\x00")
		if len(f) != 4 {
			continue
		}
		unix, _ := strconv.ParseInt(f[2], 10, 64)
		track := f[1]
		refs = append(refs, BranchRef{
			Name:         f[0],
			HasUpstream:  track != "",
			UpstreamGone: track == "[gone]",
			CommitUnix:   unix,
			Subject:      f[3],
		})
```

- [ ] **Step 5: BaseCommits 함수를 추가한다**

`apps/git-tidy/internal/gitx/gitx.go` 에 다음 함수를 추가한다.

```go
// BaseCommits 는 absorbed 판정에 쓸 base 브랜치 커밋 목록을 최신순으로 돌려준다.
func BaseCommits(base string) ([]CommitRef, error) {
	out, err := run("log", "--format=%h%x00%ct%x00%s", base)
	if err != nil {
		return nil, err
	}
	return parseCommitLines(out), nil
}
```

- [ ] **Step 6: 기존 BranchRef 파서 테스트 기대값을 수정한다**

`TestParseBranchLines` 의 input 을 subject 포함 형식으로 바꾸고, 기대값에도 `Subject` 를 넣는다.

```go
input := "feature-a\x00[gone]\x001700000000\x00[ABC-1] gone branch\n" +
	"feature-b\x00\x001800000000\x00[ABC-2] no upstream\n" +
	"feature-c\x00[ahead 1]\x001900000000\x00[ABC-3] ahead branch\n"
want := []BranchRef{
	{Name: "feature-a", UpstreamGone: true, HasUpstream: true, CommitUnix: 1700000000, Subject: "[ABC-1] gone branch"},
	{Name: "feature-b", UpstreamGone: false, HasUpstream: false, CommitUnix: 1800000000, Subject: "[ABC-2] no upstream"},
	{Name: "feature-c", UpstreamGone: false, HasUpstream: true, CommitUnix: 1900000000, Subject: "[ABC-3] ahead branch"},
}
```

- [ ] **Step 7: gitx 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/gitx
```

Expected: `ok github.com/silee-tools/git-tidy/internal/gitx`.

- [ ] **Step 8: classify 테스트도 다시 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/classify ./internal/gitx
```

Expected: both packages pass.

- [ ] **Step 9: 커밋한다**

```bash
git add apps/git-tidy/internal/gitx/gitx.go apps/git-tidy/internal/gitx/gitx_test.go apps/git-tidy/internal/classify/classify_test.go
git commit -m "feat(git-tidy): read base commits for absorbed detection"
```

## Task 4: 신호 설명 문구 공유 패키지 추가

**Files:**
- Create: `apps/git-tidy/internal/reason/reason.go`
- Test: `apps/git-tidy/internal/reason/reason_test.go`

- [ ] **Step 1: 실패하는 설명 문구 테스트를 추가한다**

`apps/git-tidy/internal/reason/reason_test.go` 를 새로 만들고 다음 내용을 넣는다.

```go
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
```

- [ ] **Step 2: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/reason
```

Expected: `apps/git-tidy/internal/reason` 패키지가 없어서 실패한다.

- [ ] **Step 3: reason 패키지를 추가한다**

`apps/git-tidy/internal/reason/reason.go` 를 새로 만들고 다음 내용을 넣는다.

```go
package reason

func Description(signal string) string {
	switch signal {
	case "gone":
		return "upstream 추적 브랜치가 사라진 로컬 브랜치"
	case "merged":
		return "base 브랜치에 브랜치 커밋이 그대로 들어간 로컬 브랜치"
	case "absorbed":
		return "같은 Jira 티켓의 더 최신 base 커밋이 있고, 지금 worktree 에서 작업 중이지 않은 로컬 브랜치"
	case "stale":
		return "마지막 커밋 또는 분기점이 stale 기준일보다 오래된 로컬 브랜치"
	default:
		return ""
	}
}
```

- [ ] **Step 4: reason 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/reason
```

Expected: `ok github.com/silee-tools/git-tidy/internal/reason`.

- [ ] **Step 5: 커밋한다**

```bash
git add apps/git-tidy/internal/reason/reason.go apps/git-tidy/internal/reason/reason_test.go
git commit -m "feat(git-tidy): share cleanup reason descriptions"
```

## Task 5: main 분류 흐름과 dry-run 출력 연결

**Files:**
- Modify: `apps/git-tidy/cmd/git-tidy/main.go`
- Modify: `apps/git-tidy/cmd/git-tidy/main_test.go`

- [ ] **Step 1: dry-run 출력 테스트를 추가한다**

`apps/git-tidy/cmd/git-tidy/main_test.go` 에 `bytes`, `strings`, `github.com/silee-tools/git-tidy/internal/classify` import 를 추가하고, 다음 테스트를 넣는다.

```go
func TestPrintTargetsShowsSignalDescriptionsAndAbsorbedEvidence(t *testing.T) {
	c := classify.Classified{
		ToDelete: []classify.Result{
			{Name: "claude/example-absorbed-branch", Signal: classify.SignalAbsorbed, AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서"},
		},
	}
	var out bytes.Buffer
	printTargetsTo(&out, c)
	s := out.String()
	for _, want := range []string{
		"[absorbed]",
		"같은 Jira 티켓의 더 최신 base 커밋",
		"base: 9a640b52f [ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("출력에 %q 가 없음:\n%s", want, s)
		}
	}
}
```

- [ ] **Step 2: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./cmd/git-tidy
```

Expected: `printTargetsTo` 가 없어서 컴파일 실패한다.

- [ ] **Step 3: buildClassification 에 base 커밋 입력을 연결한다**

`apps/git-tidy/cmd/git-tidy/main.go` 의 `buildClassification` 에 다음 호출을 추가한다.

```go
	baseCommits, err := gitx.BaseCommits(base)
	if err != nil {
		return classify.Classified{}, err
	}
```

`classify.Input` 생성에 다음 필드를 추가한다.

```go
		BaseCommits: baseCommits,
```

- [ ] **Step 4: dry-run 출력 함수를 writer 주입형으로 분리한다**

`printTargets` 를 다음처럼 감싼다.

```go
func printTargets(c classify.Classified) {
	printTargetsTo(os.Stdout, c)
}
```

그리고 기존 `printTargets` 본문을 `printTargetsTo` 로 옮긴다.

```go
func printTargetsTo(out io.Writer, c classify.Classified) {
	fmt.Fprintf(out, "삭제 대상 (%d):\n", len(c.ToDelete))
	var cur classify.Signal
	for _, r := range c.ToDelete {
		if r.Signal != cur {
			cur = r.Signal
			fmt.Fprintf(out, "  [%s]\n", cur)
			if desc := reason.Description(string(cur)); desc != "" {
				fmt.Fprintf(out, "    %s\n", desc)
			}
		}
		line := "    " + r.Name
		if r.WorktreePath != "" {
			line += "  [worktree 동반 제거: " + filepath.Base(r.WorktreePath) + "]"
		}
		if r.Signal == classify.SignalAbsorbed && r.AbsorbedByShortHash != "" {
			line += fmt.Sprintf("  (base: %s %s)", r.AbsorbedByShortHash, r.AbsorbedBySubject)
		}
		if r.AgeDays > 0 {
			line += fmt.Sprintf("  (%d일 경과)", r.AgeDays)
		}
		fmt.Fprintln(out, line)
	}
	if len(c.Excluded) > 0 {
		fmt.Fprintf(out, "제외된 후보 (%d):\n", len(c.Excluded))
		for _, e := range c.Excluded {
			fmt.Fprintf(out, "  %s  (%s)  [보호: %s]\n", e.Name, e.Signal, e.Reason)
		}
	}
	if c.OtherCount > 0 {
		fmt.Fprintf(out, "그 외 브랜치 %d개는 정리 대상이 아닙니다.\n", c.OtherCount)
	}
}
```

`main.go` import 에 `io` 와 `github.com/silee-tools/git-tidy/internal/reason` 을 추가한다.

- [ ] **Step 5: runDeletion 기본 체크 정책을 수정한다**

`runDeletion` 의 Item 생성에 absorbed 근거 필드를 넣고, 기본 체크는 계속 gone 만 true 로 둔다.

```go
items[i] = pick.Item{
	Name:                r.Name,
	Signal:              string(r.Signal),
	WorktreePath:        r.WorktreePath,
	AgeDays:             r.AgeDays,
	AbsorbedByShortHash: r.AbsorbedByShortHash,
	AbsorbedBySubject:   r.AbsorbedBySubject,
	Checked:             r.Signal == classify.SignalGone,
}
```

- [ ] **Step 6: main 패키지 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./cmd/git-tidy
```

Expected: `ok github.com/silee-tools/git-tidy/cmd/git-tidy`.

- [ ] **Step 7: 커밋한다**

```bash
git add apps/git-tidy/cmd/git-tidy/main.go apps/git-tidy/cmd/git-tidy/main_test.go
git commit -m "feat(git-tidy): show absorbed branch evidence"
```

## Task 6: pick 모델과 줄 기반 선택 화면 확장

**Files:**
- Modify: `apps/git-tidy/internal/pick/model.go`
- Modify: `apps/git-tidy/internal/pick/model_test.go`
- Modify: `apps/git-tidy/internal/pick/line.go`
- Modify: `apps/git-tidy/internal/pick/line_test.go`

- [ ] **Step 1: pick Item 필드 보존 테스트를 수정한다**

`apps/git-tidy/internal/pick/model_test.go` 의 `items()` 에 absorbed 항목을 `merged` 와 `stale` 사이에 추가한다.

```go
{Name: "a1", Signal: "absorbed", AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: absorbed base", Checked: false},
```

`TestGroupsInOrder` 기대값을 다음처럼 바꾼다.

```go
[]string{"gone", "merged", "absorbed", "stale"}
```

`TestItemFieldsPreserved` 에 다음 확인을 추가한다.

```go
got := m.Items()
if got[3].AbsorbedByShortHash != "9a640b52f" || got[3].AbsorbedBySubject == "" {
	t.Errorf("absorbed 근거 필드 보존 실패: %+v", got[3])
}
if got[4].AgeDays != 34 || got[4].Signal != "stale" {
	t.Errorf("stale Item 필드 보존 실패: %+v", got[4])
}
```

- [ ] **Step 2: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/pick
```

Expected: `AbsorbedByShortHash` 필드가 없어서 컴파일 실패한다.

- [ ] **Step 3: pick.Item 에 absorbed 근거 필드를 추가한다**

`apps/git-tidy/internal/pick/model.go` 의 `Item` 을 확장한다.

```go
type Item struct {
	Name                string
	Signal              string
	WorktreePath        string
	AgeDays             int
	AbsorbedByShortHash string
	AbsorbedBySubject   string
	Checked             bool
}
```

- [ ] **Step 4: 줄 기반 출력 테스트에 설명과 근거를 추가한다**

`apps/git-tidy/internal/pick/line_test.go` 의 `lineItems()` 에 absorbed 항목을 추가한다.

```go
{Name: "a1", Signal: "absorbed", AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: absorbed base", Checked: false},
```

`TestRunLineRendersGroupsAndMeta` 기대 문자열에 다음 값을 추가한다.

```go
"absorbed (1)",
"같은 Jira 티켓의 더 최신 base 커밋",
"base: 9a640b52f [ABC-1375] feat: absorbed base",
```

- [ ] **Step 5: line.go 에 설명과 absorbed 근거 렌더링을 추가한다**

`apps/git-tidy/internal/pick/line.go` 에 `github.com/silee-tools/git-tidy/internal/reason` import 를 추가한다.

`renderLine` 의 그룹 헤더 출력 직후 설명을 출력한다.

```go
			if desc := reason.Description(cur); desc != "" {
				_, _ = fmt.Fprintf(out, "     %s\n", desc)
			}
```

항목 라인에 absorbed 근거를 붙인다.

```go
		if it.Signal == "absorbed" && it.AbsorbedByShortHash != "" {
			line += fmt.Sprintf("   base: %s %s", it.AbsorbedByShortHash, it.AbsorbedBySubject)
		}
```

- [ ] **Step 6: pick 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/pick
```

Expected: `ok github.com/silee-tools/git-tidy/internal/pick`.

- [ ] **Step 7: 커밋한다**

```bash
git add apps/git-tidy/internal/pick/model.go apps/git-tidy/internal/pick/model_test.go apps/git-tidy/internal/pick/line.go apps/git-tidy/internal/pick/line_test.go
git commit -m "feat(git-tidy): render absorbed branches in line picker"
```

## Task 7: TUI 설명과 absorbed 표시 추가

**Files:**
- Modify: `apps/git-tidy/internal/pick/tui.go`
- Modify: `apps/git-tidy/internal/pick/tui_test.go`

- [ ] **Step 1: TUI 테스트 데이터에 absorbed 를 추가한다**

`apps/git-tidy/internal/pick/tui_test.go` 의 `tuiItems()` 에 다음 항목을 추가한다.

```go
{Name: "a1", Signal: "absorbed", AbsorbedByShortHash: "9a640b52f", AbsorbedBySubject: "[ABC-1375] feat: absorbed base", Checked: false},
```

`TestTUIRowsLayout` 의 row 개수와 주석을 `[헤더 gone, g1, g2, 헤더 merged, m1, 헤더 absorbed, a1]` 기준으로 수정한다.

- [ ] **Step 2: View 설명 렌더링 테스트를 추가한다**

`apps/git-tidy/internal/pick/tui_test.go` 끝에 다음 테스트를 추가한다.

```go
func TestTUIViewShowsDescriptionsAndAbsorbedEvidence(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	s := m.View()
	for _, want := range []string{
		"같은 Jira 티켓의 더 최신 base 커밋",
		"base: 9a640b52f [ABC-1375] feat: absorbed base",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("TUI 출력에 %q 가 없음:\n%s", want, s)
		}
	}
}
```

`tui_test.go` import 에 `strings` 를 추가한다.

- [ ] **Step 3: Red 를 확인한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/pick
```

Expected: TUI 출력에 absorbed 설명과 근거가 없어서 실패한다.

- [ ] **Step 4: TUI 색과 헤더 설명을 확장한다**

`apps/git-tidy/internal/pick/tui.go` 의 `signalColors` 를 수정하고, `headerHint` 값을 `reason.Description` 으로 채운다. `tui.go` import 에 `github.com/silee-tools/git-tidy/internal/reason` 을 추가한다.

```go
signalColors = map[string]lipgloss.Color{
	"gone":     lipgloss.Color("9"),
	"merged":   lipgloss.Color("10"),
	"absorbed": lipgloss.Color("14"),
	"stale":    lipgloss.Color("11"),
}
headerHint = map[string]string{
	"gone":     reason.Description("gone"),
	"merged":   reason.Description("merged"),
	"absorbed": reason.Description("absorbed"),
	"stale":    reason.Description("stale"),
}
```

- [ ] **Step 5: TUI 항목 렌더링에 absorbed 근거를 추가한다**

`renderRow` 의 항목 라인과 `itemPlain` 에 다음 조건을 추가한다.

```go
	if it.Signal == "absorbed" && it.AbsorbedByShortHash != "" {
		line += styleDim.Render(fmt.Sprintf("   base: %s %s", it.AbsorbedByShortHash, it.AbsorbedBySubject))
	}
```

`itemPlain` 에는 스타일 없이 추가한다.

```go
	if it.Signal == "absorbed" && it.AbsorbedByShortHash != "" {
		line += fmt.Sprintf("   base: %s %s", it.AbsorbedByShortHash, it.AbsorbedBySubject)
	}
```

- [ ] **Step 6: pick 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./internal/pick
```

Expected: `ok github.com/silee-tools/git-tidy/internal/pick`.

- [ ] **Step 7: 커밋한다**

```bash
git add apps/git-tidy/internal/pick/tui.go apps/git-tidy/internal/pick/tui_test.go
git commit -m "feat(git-tidy): render absorbed branches in tui"
```

## Task 8: 전체 검증과 large-repo fixture 재현

**Files:**
- 앞선 작업에서 형식이나 import 정리만 남았을 때 해당 파일을 수정한다.

- [ ] **Step 1: 전체 테스트를 실행한다**

Run:

```bash
cd apps/git-tidy
go test ./...
```

Expected: all packages pass.

- [ ] **Step 2: gofmt 를 실행한다**

Run:

```bash
cd apps/git-tidy
gofmt -w cmd internal
git diff --check
```

Expected: `git diff --check` prints nothing and exits 0.

- [ ] **Step 3: 임시 바이너리를 세션 전용 디렉터리에 빌드한다**

Run:

```bash
cd apps/git-tidy
WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-git-tidy-absorbed}"
mkdir -p "$WORK"
go build -o "$WORK/git-tidy" ./cmd/git-tidy
```

Expected: build exits 0 and creates `$WORK/git-tidy`.

- [ ] **Step 4: large-repo fixture 에서 dry-run 을 확인한다**

Run:

```bash
cd <large-repo fixture checkout>
"${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-git-tidy-absorbed}/git-tidy" --no-fetch --stale-days=20
```

Expected: 출력에 `[absorbed]` 그룹이 있고, `claude/example-absorbed-branch` 가 포함된다. 해당 줄은 `base: 9a640b52f` 같은 근거 커밋을 함께 보여준다.

- [ ] **Step 5: 줄 기반 선택 화면의 기본 체크를 확인한다**

Run:

```bash
cd <large-repo fixture checkout>
printf 'q\n' | "${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-git-tidy-absorbed}/git-tidy" --run --no-tui --no-fetch --stale-days=20
```

Expected: `[absorbed]` 그룹의 항목은 `[ ]` 로 보이고, `[x]` 로 기본 체크되어 있지 않다.

- [ ] **Step 6: 최종 상태를 확인한다**

Run:

```bash
git status -sb
/usr/bin/python3 scripts/check-commit-messages.py --from fba3f3a --to HEAD
```

Expected: 작업 파일만 의도한 범위로 남아 있고, 현재 작업 커밋들의 Conventional Commit 헤더가 모두 통과한다.

- [ ] **Step 7: 최종 커밋을 만든다**

Task 8 에서 gofmt 또는 작은 import 정리만 발생했다면 다음으로 커밋한다.

```bash
git add apps/git-tidy
git commit -m "chore(git-tidy): verify absorbed branch cleanup flow"
```

변경이 없다면 커밋하지 않는다.
