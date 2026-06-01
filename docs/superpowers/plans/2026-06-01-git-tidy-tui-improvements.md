# git-tidy TUI 개선 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git-tidy 의 삭제 후보를 사유별로 정렬·그룹핑하고, `[gone]`만 기본 체크하며, bubbletea 기반 TUI 에 그룹 헤더·그룹 토글·worktree 이름·stale 경과 일수를 표시한다.

**Architecture:** 정렬과 경과 일수 계산은 순수 함수 `classify` 에서 한다. `pick.Selection` 모델을 그룹·worktree·경과·초기 체크를 아는 구조로 바꾸고, 그 위에 bubbletea TUI 와 줄 기반 폴백을 둔다. `main.go` 는 `classify.Result` 를 `pick.Item` 으로 변환해 모델을 만들고, dry-run 출력도 그룹별로 바꾼다.

**Tech Stack:** Go 1.23, charmbracelet/bubbletea + lipgloss (신규), golang.org/x/term (기존), 표준 testing.

**작업 디렉터리:** 모든 명령은 `apps/git-tidy/` 기준. 테스트는 `cd apps/git-tidy && go test ./...`.

---

## File Structure

- `internal/classify/classify.go` — Result 에 `AgeDays` 추가, 신호별 정렬, 경과 일수 계산.
- `internal/classify/classify_test.go` — 정렬·경과 일수 검증 보강.
- `internal/pick/model.go` — `Item` 구조체 도입, `NewSelection([]Item)`, 그룹 연산.
- `internal/pick/model_test.go` — 새 API·기본 체크·그룹 토글 검증.
- `internal/pick/line.go` — 그룹 헤더·worktree·경과 표시, labels 파라미터 제거.
- `internal/pick/line_test.go` — 새 시그니처·렌더링 검증.
- `internal/pick/tui.go` — bubbletea 재작성.
- `internal/pick/tui_test.go` — (신규) Update 순수 함수 단위 테스트.
- `cmd/git-tidy/main.go` — Result→Item 변환, RunTUI/RunLine 호출 변경, printTargets 그룹화.
- `go.mod` / `go.sum` — bubbletea·lipgloss 추가.
- `README.md` / `apps/git-tidy/CLAUDE.md` — 동작·모델 설명 갱신.

---

## Task 1: classify — 신호별 정렬과 stale 경과 일수

**Files:**
- Modify: `apps/git-tidy/internal/classify/classify.go`
- Test: `apps/git-tidy/internal/classify/classify_test.go`

- [ ] **Step 1: 기존 테스트를 새 기대값으로 고쳐 실패시키기 (Red)**

`classify_test.go` 의 `wantDelete` 를 stale 항목에 경과 일수가 붙도록 바꾼다. 기존 테스트에서 `feature-stale` 의 `CommitUnix` 는 `old = now - 30*day` 이므로 경과 30일이다. `internal/classify/classify_test.go` 의 `wantDelete` 블록을 아래로 교체한다:

```go
	wantDelete := []Result{
		{Name: "feature-gone", Signal: SignalGone},
		{Name: "feature-merged", Signal: SignalMerged},
		{Name: "feature-stale", Signal: SignalStale, AgeDays: 30},
	}
```

그리고 같은 파일 끝에 정렬 검증 테스트를 추가한다(이름 역순으로 들어온 stale 들이 이름순으로 정렬되는지):

```go
func TestClassifySortsBySignalThenName(t *testing.T) {
	now := int64(1_000_000_000)
	day := int64(86400)
	old := now - 40*day
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "zzz-stale", HasUpstream: true, CommitUnix: old},
			{Name: "aaa-stale", HasUpstream: true, CommitUnix: old},
			{Name: "mmm-gone", UpstreamGone: true, HasUpstream: true, CommitUnix: now},
		},
		Merged:        map[string]bool{},
		Worktrees:     map[string]string{},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}
	got := Classify(in)
	want := []Result{
		{Name: "mmm-gone", Signal: SignalGone},
		{Name: "aaa-stale", Signal: SignalStale, AgeDays: 40},
		{Name: "zzz-stale", Signal: SignalStale, AgeDays: 40},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("정렬 결과 mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}
```

- [ ] **Step 2: 실패 확인 (Red 관찰)**

Run: `cd apps/git-tidy && go test ./internal/classify/ -run 'TestClassify' -v`
Expected: FAIL — `AgeDays` 필드가 없어 컴파일 에러는 나지 않지만(필드 미존재면 컴파일 에러), 실제로는 `Result` 에 `AgeDays` 가 없어 컴파일 실패. 먼저 Step 3 의 필드 추가 없이 돌리면 컴파일 에러가 난다. 컴파일이 통과하도록 필드만 먼저 본다면 값이 0이라 mismatch 로 실패한다. 여기서는 컴파일 에러(필드 없음)를 Red 신호로 인정하지 말고, Step 3 에서 필드만 추가한 뒤(정렬·계산 미구현) 다시 돌려 값 mismatch 실패를 눈으로 확인한다.

- [ ] **Step 3: Result 필드만 추가하고 다시 실패 관찰**

`classify.go` 의 `Result` 에 필드를 추가한다:

```go
// Result 는 삭제 대상 브랜치 하나다.
type Result struct {
	Name         string
	Signal       Signal
	WorktreePath string // worktree 에 물려 있으면 그 경로, 아니면 빈 문자열
	AgeDays      int    // stale 일 때 마지막 커밋 기준 경과 일수, 그 외 0
}
```

Run: `cd apps/git-tidy && go test ./internal/classify/ -run 'TestClassify' -v`
Expected: FAIL — `AgeDays` 가 0이라 stale 기대값(30/40)과 mismatch, 정렬 테스트도 입력 순서(zzz,aaa)대로라 mismatch.

- [ ] **Step 4: 정렬과 경과 일수 구현 (Green)**

`classify.go` 상단 import 에 `"sort"` 를 추가한다:

```go
import (
	"sort"

	"github.com/silee-tools/git-tidy/internal/gitx"
)
```

`Classify` 의 ToDelete append 부분을 경과 일수까지 채우도록 바꾸고, 루프 뒤에 정렬을 추가한다. `Classify` 함수의 마지막 append 블록과 return 사이를 아래로 교체한다:

```go
		out.ToDelete = append(out.ToDelete, Result{
			Name:         b.Name,
			Signal:       sig,
			WorktreePath: in.Worktrees[b.Name],
			AgeDays:      ageDaysFor(b, in, sig),
		})
	}
	sort.SliceStable(out.ToDelete, func(i, j int) bool {
		ri, rj := signalRank(out.ToDelete[i].Signal), signalRank(out.ToDelete[j].Signal)
		if ri != rj {
			return ri < rj
		}
		return out.ToDelete[i].Name < out.ToDelete[j].Name
	})
	return out
}

// signalRank 는 신호의 정렬 순위다(확실한 순: gone < merged < stale).
func signalRank(s Signal) int {
	switch s {
	case SignalGone:
		return 0
	case SignalMerged:
		return 1
	case SignalStale:
		return 2
	default:
		return 3
	}
}

// ageDaysFor 는 stale 후보의 마지막 커밋 기준 경과 일수를 돌려준다.
// 커밋 시각 정보가 없으면 merge-base 시각으로 폴백하고, 둘 다 없으면 0이다.
// stale 이 아니면 0이다.
func ageDaysFor(b gitx.BranchRef, in Input, sig Signal) int {
	if sig != SignalStale {
		return 0
	}
	ts := b.CommitUnix
	if ts == 0 {
		if mb, ok := in.MergeBaseUnix(b.Name); ok {
			ts = mb
		}
	}
	if ts == 0 {
		return 0
	}
	return int((in.Now - ts) / 86400)
}
```

- [ ] **Step 5: 통과 확인 (Green)**

Run: `cd apps/git-tidy && go test ./internal/classify/ -v`
Expected: PASS (TestClassify, TestClassifySortsBySignalThenName 모두 통과)

- [ ] **Step 6: Commit**

```bash
cd apps/git-tidy && git add internal/classify/
git commit -m "feat(git-tidy): 삭제 후보를 신호별 정렬하고 stale 경과 일수 계산

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: pick 모델 — Item 구조체, gone 기본 체크, 그룹 토글

**Files:**
- Modify: `apps/git-tidy/internal/pick/model.go`
- Test: `apps/git-tidy/internal/pick/model_test.go`

- [ ] **Step 1: 새 API 로 테스트 재작성 (Red)**

`model_test.go` 전체를 아래로 교체한다:

```go
package pick

import (
	"reflect"
	"testing"
)

func items() []Item {
	return []Item{
		{Name: "g1", Signal: "gone", Checked: true},
		{Name: "g2", Signal: "gone", Checked: true},
		{Name: "m1", Signal: "merged", Checked: false},
		{Name: "s1", Signal: "stale", AgeDays: 34, Checked: false},
	}
}

func TestNewSelectionRespectsInitialChecked(t *testing.T) {
	m := NewSelection(items())
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"g1", "g2"}) {
		t.Errorf("초기 체크는 gone 항목만이어야 함, got %v", got)
	}
}

func TestGroupsInOrder(t *testing.T) {
	m := NewSelection(items())
	if got := m.Groups(); !reflect.DeepEqual(got, []string{"gone", "merged", "stale"}) {
		t.Errorf("그룹 순서 mismatch, got %v", got)
	}
}

func TestToggleGroupScopedToSignal(t *testing.T) {
	m := NewSelection(items())
	m.ToggleGroup("merged") // merged 안에 켜진 게 없으므로 전체 체크
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"g1", "g2", "m1"}) {
		t.Errorf("ToggleGroup(merged) 후 %v, want [g1 g2 m1]", got)
	}
	m.ToggleGroup("gone") // gone 둘 다 켜져 있으므로 전체 해제
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"m1"}) {
		t.Errorf("ToggleGroup(gone) 후 %v, want [m1]", got)
	}
}

func TestItemFieldsPreserved(t *testing.T) {
	m := NewSelection(items())
	got := m.Items()
	if got[3].AgeDays != 34 || got[3].Signal != "stale" {
		t.Errorf("Item 필드 보존 실패: %+v", got[3])
	}
}

func TestToggleAllStillWorks(t *testing.T) {
	m := NewSelection(items())
	m.ToggleAll() // 하나라도 켜져 있으면 전체 해제
	if got := m.Checked(); len(got) != 0 {
		t.Errorf("ToggleAll 후 전체 해제 기대, got %v", got)
	}
	m.ToggleAll() // 전부 꺼져 있으면 전체 체크
	if got := m.Checked(); len(got) != 4 {
		t.Errorf("ToggleAll 재호출 후 전체 체크 기대, got %v", got)
	}
}
```

- [ ] **Step 2: 실패 확인 (Red)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -run 'TestNewSelection|TestGroups|TestToggleGroup|TestItemFields' -v`
Expected: FAIL — 컴파일 에러(`Item` 미정의, `NewSelection` 시그니처 불일치, `Groups`/`ToggleGroup` 미존재). 컴파일이 되도록 Step 3 의 타입만 먼저 추가한 뒤(메서드 미구현) 다시 돌려 동작 실패를 확인하는 것이 이상적이나, Go 는 부분 구현 시 미존재 메서드가 컴파일 에러이므로, Step 3 에서 전체 구현 후 Green 을 관찰한다. (Red 는 기존 `[]string` API 와의 시그니처 충돌로 확인된다.)

- [ ] **Step 3: 모델 재구현 (Green)**

`model.go` 전체를 아래로 교체한다:

```go
// Package pick 는 삭제 대상 다중 선택을 담당한다. 순수 선택 모델 위에
// 체크박스 TUI 와 줄 기반 선택 두 front-end 를 둔다.
package pick

// Item 은 선택 대상 하나의 표시 정보와 초기 체크 상태다.
type Item struct {
	Name         string
	Signal       string // 삭제 사유(그룹 키): gone / merged / stale
	WorktreePath string // worktree 에 물려 있으면 그 경로, 아니면 빈 문자열
	AgeDays      int    // stale 경과 일수, 그 외 0
	Checked      bool   // 초기 체크 상태
}

// Selection 은 항목별 체크 상태를 들고 있는 순수 모델이다.
// 렌더링·입력과 분리돼 있어 TUI 와 줄 기반 모드가 함께 쓰고 단위 테스트한다.
type Selection struct {
	items   []Item
	checked []bool
}

// NewSelection 은 각 Item 의 초기 Checked 를 반영한 모델을 만든다.
func NewSelection(items []Item) *Selection {
	checked := make([]bool, len(items))
	for i, it := range items {
		checked[i] = it.Checked
	}
	return &Selection{items: items, checked: checked}
}

// Items 는 전체 항목을 돌려준다.
func (s *Selection) Items() []Item { return s.items }

// IsChecked 는 i 번째 항목의 체크 여부다.
func (s *Selection) IsChecked(i int) bool { return s.checked[i] }

// Toggle 은 i 번째 항목의 체크를 뒤집는다.
func (s *Selection) Toggle(i int) {
	if i >= 0 && i < len(s.checked) {
		s.checked[i] = !s.checked[i]
	}
}

// ToggleAll 은 하나라도 체크돼 있으면 전체 해제, 아니면 전체 체크한다.
func (s *Selection) ToggleAll() {
	anyChecked := false
	for _, c := range s.checked {
		if c {
			anyChecked = true
			break
		}
	}
	for i := range s.checked {
		s.checked[i] = !anyChecked
	}
}

// Groups 는 항목 등장 순서대로 중복 없는 그룹(신호) 키를 돌려준다.
func (s *Selection) Groups() []string {
	var out []string
	seen := map[string]bool{}
	for _, it := range s.items {
		if !seen[it.Signal] {
			seen[it.Signal] = true
			out = append(out, it.Signal)
		}
	}
	return out
}

// ToggleGroup 은 한 그룹 안에 하나라도 체크돼 있으면 그 그룹 전체 해제,
// 아니면 그 그룹 전체 체크한다(ToggleAll 규칙의 그룹 범위판).
func (s *Selection) ToggleGroup(signal string) {
	anyChecked := false
	for i, it := range s.items {
		if it.Signal == signal && s.checked[i] {
			anyChecked = true
			break
		}
	}
	for i, it := range s.items {
		if it.Signal == signal {
			s.checked[i] = !anyChecked
		}
	}
}

// Checked 는 체크된 항목 이름들을 순서대로 돌려준다.
func (s *Selection) Checked() []string {
	var out []string
	for i, c := range s.checked {
		if c {
			out = append(out, s.items[i].Name)
		}
	}
	return out
}
```

- [ ] **Step 4: 통과 확인 (Green)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -run 'TestNewSelection|TestGroups|TestToggleGroup|TestItemFields|TestToggleAll' -v`
Expected: PASS (line/tui 관련 테스트는 아직 깨진 상태일 수 있음 — Task 3, 4 에서 고친다)

- [ ] **Step 5: Commit**

```bash
cd apps/git-tidy && git add internal/pick/model.go internal/pick/model_test.go
git commit -m "feat(git-tidy): pick 모델에 그룹·초기체크·worktree·경과 도입

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: 줄 모드 — 그룹 헤더·worktree·경과 표시

**Files:**
- Modify: `apps/git-tidy/internal/pick/line.go`
- Test: `apps/git-tidy/internal/pick/line_test.go`

- [ ] **Step 1: 새 시그니처로 테스트 재작성 (Red)**

`line_test.go` 전체를 아래로 교체한다. `RunLine` 에서 labels 파라미터를 제거하고, 모델이 직접 표시 정보를 들고 있다:

```go
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
	for _, want := range []string{"gone (1)", "merged (1)", "stale (1)", "34일 경과", "⌂ s1"} {
		if !strings.Contains(s, want) {
			t.Errorf("출력에 %q 가 없음:\n%s", want, s)
		}
	}
}
```

- [ ] **Step 2: 실패 확인 (Red)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -run 'TestRunLine' -v`
Expected: FAIL — `RunLine` 시그니처가 아직 labels 를 받으므로 컴파일/인자 불일치.

- [ ] **Step 3: line.go 재구현 (Green)**

`line.go` 전체를 아래로 교체한다:

```go
package pick

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

// RunLine 은 줄 입력으로 다중 선택을 진행하고 체크된 항목을 돌려준다.
// 두 번째 반환값이 false 면 사용자가 취소한 것이다. 표시 정보(신호·worktree·
// 경과)는 Selection 의 Item 이 들고 있다. in/out 은 테스트를 위해 주입한다.
func RunLine(sel *Selection, in io.Reader, out io.Writer) ([]string, bool) {
	r := bufio.NewScanner(in)
	for {
		renderLine(sel, out)
		_, _ = fmt.Fprint(out, "번호=토글, a=전체토글, 빈 줄=완료, q=취소 > ")
		if !r.Scan() {
			return nil, false
		}
		switch cmd := strings.TrimSpace(r.Text()); cmd {
		case "":
			return confirmLine(sel, r, out)
		case "q":
			return nil, false
		case "a":
			sel.ToggleAll()
		default:
			if n, err := strconv.Atoi(cmd); err == nil {
				sel.Toggle(n - 1)
			}
		}
	}
}

func groupCount(sel *Selection, signal string) int {
	n := 0
	for _, it := range sel.Items() {
		if it.Signal == signal {
			n++
		}
	}
	return n
}

func renderLine(sel *Selection, out io.Writer) {
	cur := ""
	for i, it := range sel.Items() {
		if it.Signal != cur {
			cur = it.Signal
			_, _ = fmt.Fprintf(out, "  ── %s (%d) ──\n", cur, groupCount(sel, cur))
		}
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		line := fmt.Sprintf("  %2d. [%s] %s", i+1, mark, it.Name)
		if it.WorktreePath != "" {
			line += "  ⌂ " + filepath.Base(it.WorktreePath)
		}
		if it.AgeDays > 0 {
			line += fmt.Sprintf("   %d일 경과", it.AgeDays)
		}
		_, _ = fmt.Fprintln(out, line)
	}
}

func confirmLine(sel *Selection, r *bufio.Scanner, out io.Writer) ([]string, bool) {
	checked := sel.Checked()
	if len(checked) == 0 {
		return nil, true
	}
	_, _ = fmt.Fprintf(out, "%d개 브랜치를 삭제합니다. 진행할까요? [y/N] ", len(checked))
	if !r.Scan() {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(r.Text()), "y") {
		return checked, true
	}
	return nil, false
}
```

- [ ] **Step 4: 통과 확인 (Green)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -run 'TestRunLine' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd apps/git-tidy && git add internal/pick/line.go internal/pick/line_test.go
git commit -m "feat(git-tidy): 줄 모드에 그룹 헤더·worktree·경과 표시

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: bubbletea TUI 재작성

**Files:**
- Modify: `apps/git-tidy/internal/pick/tui.go`
- Create: `apps/git-tidy/internal/pick/tui_test.go`
- Modify: `apps/git-tidy/go.mod`, `apps/git-tidy/go.sum`

- [ ] **Step 1: 의존성 추가**

Run:
```bash
cd apps/git-tidy && go get github.com/charmbracelet/bubbletea@latest && go get github.com/charmbracelet/lipgloss@latest
```
Expected: go.mod 에 두 require 가 추가되고 go.sum 갱신.

- [ ] **Step 2: Update 순수 함수 테스트 작성 (Red)**

`tui_test.go` 를 새로 만든다. bubbletea 의 `Model.Update` 에 키 메시지를 주입해 상태 전이를 검증한다:

```go
package pick

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tuiItems() []Item {
	return []Item{
		{Name: "g1", Signal: "gone", Checked: true},
		{Name: "g2", Signal: "gone", Checked: true},
		{Name: "m1", Signal: "merged", Checked: false},
	}
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// rows 는 [헤더 gone, g1, g2, 헤더 merged, m1] 순서다(인덱스 0..4).
func TestTUIRowsLayout(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	if len(m.rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(m.rows))
	}
	if !m.rows[0].isHeader || m.rows[0].signal != "gone" {
		t.Errorf("rows[0] 은 gone 헤더여야 함: %+v", m.rows[0])
	}
	if m.rows[1].isHeader || m.rows[1].itemIdx != 0 {
		t.Errorf("rows[1] 은 g1 항목이어야 함: %+v", m.rows[1])
	}
}

func TestTUIToggleItemOnSpace(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	// 커서를 rows[1](g1) 로 내리고 space 로 해제
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.sel.IsChecked(0) {
		t.Error("space 로 g1 이 해제돼야 함")
	}
}

func TestTUIToggleGroupOnHeader(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	// 커서는 rows[0](gone 헤더). gone 둘 다 켜져 있으므로 space 로 전체 해제.
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.sel.IsChecked(0) || m.sel.IsChecked(1) {
		t.Error("gone 헤더 space 로 그룹 전체가 해제돼야 함")
	}
}

func TestTUIEnterConfirms(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done || m.cancel {
		t.Errorf("enter 는 확정(done=true, cancel=false)이어야 함: done=%v cancel=%v", m.done, m.cancel)
	}
}

func TestTUIEscCancels(t *testing.T) {
	m := newTUIModel(NewSelection(tuiItems()))
	m, _ = updateForTest(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.cancel {
		t.Error("esc 는 취소여야 함")
	}
}
```

여기서 `updateForTest` 는 `Model.Update` 의 반환을 구체 타입으로 되돌리는 테스트 헬퍼다. 같은 파일에 추가한다:

```go
func updateForTest(m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(tuiModel), cmd
}
```

- [ ] **Step 3: 실패 확인 (Red)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -run 'TestTUI' -v`
Expected: FAIL — `newTUIModel`, `tuiModel`, `row` 미정의로 컴파일 실패.

- [ ] **Step 4: tui.go 재구현 (Green)**

`tui.go` 전체를 아래로 교체한다:

```go
package pick

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// row 는 화면 한 줄이다. 그룹 헤더이거나 항목이다.
type row struct {
	isHeader bool
	signal   string // 헤더면 그룹 키, 항목이면 그 항목의 신호
	itemIdx  int    // 항목이면 sel.items 인덱스, 헤더면 -1
}

type tuiModel struct {
	sel    *Selection
	rows   []row
	cursor int
	height int // 화면 높이(WindowSizeMsg). 0이면 전체 렌더.
	done   bool
	cancel bool
}

func buildRows(sel *Selection) []row {
	var rows []row
	cur := ""
	for i, it := range sel.Items() {
		if it.Signal != cur {
			cur = it.Signal
			rows = append(rows, row{isHeader: true, signal: cur, itemIdx: -1})
		}
		rows = append(rows, row{signal: it.Signal, itemIdx: i})
	}
	return rows
}

func newTUIModel(sel *Selection) tuiModel {
	return tuiModel{sel: sel, rows: buildRows(sel), cursor: 0}
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
			m.cancel, m.done = true, true
			return m, tea.Quit
		case msg.Type == tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case msg.Type == tea.KeyUp:
			m.moveCursor(-1)
		case msg.Type == tea.KeyDown:
			m.moveCursor(1)
		case msg.Type == tea.KeySpace:
			m.toggleAtCursor()
		case msg.Type == tea.KeyRunes && len(msg.Runes) == 1:
			switch msg.Runes[0] {
			case 'k':
				m.moveCursor(-1)
			case 'j':
				m.moveCursor(1)
			case 'a':
				m.sel.ToggleAll()
			case 'q':
				m.cancel, m.done = true, true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *tuiModel) moveCursor(delta int) {
	n := len(m.rows)
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > n-1 {
		m.cursor = n - 1
	}
}

func (m *tuiModel) toggleAtCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isHeader {
		m.sel.ToggleGroup(r.signal)
	} else {
		m.sel.Toggle(r.itemIdx)
	}
}

var (
	styleTitle  = lipgloss.NewStyle().Bold(true)
	styleHelp   = lipgloss.NewStyle().Faint(true)
	styleDim    = lipgloss.NewStyle().Faint(true)
	styleCursor = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerColors = map[string]lipgloss.Style{
		"gone":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
		"merged": lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
		"stale":  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
	}
	headerHint = map[string]string{
		"gone":   "← upstream 사라짐 · PR 머지 후",
		"merged": "← base 에 이미 합쳐짐",
		"stale":  "← 오래 경과",
	}
)

func headerStyle(signal string) lipgloss.Style {
	if s, ok := headerColors[signal]; ok {
		return s
	}
	return lipgloss.NewStyle().Bold(true)
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(
		fmt.Sprintf("git-tidy — 삭제할 브랜치 선택  (%d/%d 선택됨)",
			len(m.sel.Checked()), len(m.sel.Items()))) + "\n\n")

	start, end := m.window()
	for idx := start; idx < end; idx++ {
		b.WriteString(m.renderRow(idx) + "\n")
	}
	b.WriteString("\n" + styleHelp.Render(
		"↑↓/jk 이동 · space 토글 · a 전체 · enter 삭제 · esc 취소"))
	return b.String()
}

// window 는 화면 높이에 맞춰 그릴 row 범위를 돌려준다. height 가 0이면 전체.
func (m tuiModel) window() (int, int) {
	n := len(m.rows)
	visible := m.height - 4 // 제목 2줄 + 안내 2줄
	if m.height == 0 || visible >= n || visible < 1 {
		return 0, n
	}
	start := m.cursor - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > n {
		end = n
		start = end - visible
	}
	return start, end
}

func (m tuiModel) renderRow(idx int) string {
	r := m.rows[idx]
	cursor := "  "
	if idx == m.cursor {
		cursor = "› "
	}
	if r.isHeader {
		count := groupCount(m.sel, r.signal)
		head := fmt.Sprintf("▾ %s (%d)", r.signal, count)
		line := cursor + headerStyle(r.signal).Render(head) + "  " + styleDim.Render(headerHint[r.signal])
		if idx == m.cursor {
			return styleCursor.Render(strings.TrimRight(line, " "))
		}
		return line
	}
	it := m.sel.Items()[r.itemIdx]
	box := "◯"
	if m.sel.IsChecked(r.itemIdx) {
		box = styleChecked.Render("◉")
	}
	line := fmt.Sprintf("%s  %s %s", cursor, box, it.Name)
	if it.WorktreePath != "" {
		line += styleDim.Render("   ⌂ "+filepath.Base(it.WorktreePath)) + styleDim.Render("  [worktree 동반 제거]")
	}
	if it.AgeDays > 0 {
		line += styleDim.Render(fmt.Sprintf("   %d일 경과", it.AgeDays))
	}
	if idx == m.cursor {
		return styleCursor.Render(line)
	}
	return line
}

// RunTUI 는 bubbletea 체크박스 목록으로 다중 선택을 진행한다.
// 두 번째 반환값이 false 면 취소다. TTY 가 아니어서 프로그램 시작에 실패하면
// ok=false, 세 번째 반환값(fellBack)이 true 가 되어 호출자가 줄 기반으로 폴백한다.
func RunTUI(sel *Selection) (checked []string, ok bool, fellBack bool) {
	m := newTUIModel(sel)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, false, true
	}
	fm, _ := final.(tuiModel)
	if fm.cancel {
		return nil, false, false
	}
	return sel.Checked(), true, false
}
```

- [ ] **Step 5: 통과 확인 (Green)**

Run: `cd apps/git-tidy && go test ./internal/pick/ -v`
Expected: PASS (model·line·tui 전부)

- [ ] **Step 6: Commit**

```bash
cd apps/git-tidy && git add internal/pick/tui.go internal/pick/tui_test.go go.mod go.sum
git commit -m "feat(git-tidy): bubbletea 기반 그룹 TUI 로 선택 화면 재작성

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: main.go 배선 — Result→Item 변환, 출력 그룹화

**Files:**
- Modify: `apps/git-tidy/cmd/git-tidy/main.go`

- [ ] **Step 1: printTargets 를 그룹별로 바꾸기**

`main.go` 의 `printTargets` 함수를 아래로 교체한다(상단 import 에 `"path/filepath"` 는 이미 있음):

```go
func printTargets(c classify.Classified) {
	fmt.Printf("삭제 대상 (%d):\n", len(c.ToDelete))
	var cur classify.Signal
	for _, r := range c.ToDelete {
		if r.Signal != cur {
			cur = r.Signal
			fmt.Printf("  [%s]\n", cur)
		}
		line := "    " + r.Name
		if r.WorktreePath != "" {
			line += "  [worktree 동반 제거: " + filepath.Base(r.WorktreePath) + "]"
		}
		if r.AgeDays > 0 {
			line += fmt.Sprintf("  (%d일 경과)", r.AgeDays)
		}
		fmt.Println(line)
	}
	if len(c.Excluded) > 0 {
		fmt.Printf("제외된 후보 (%d):\n", len(c.Excluded))
		for _, e := range c.Excluded {
			fmt.Printf("  %s  (%s)  [보호: %s]\n", e.Name, e.Signal, e.Reason)
		}
	}
	if c.OtherCount > 0 {
		fmt.Printf("그 외 브랜치 %d개는 정리 대상이 아닙니다.\n", c.OtherCount)
	}
}
```

- [ ] **Step 2: runDeletion 을 새 pick API 로 배선**

`main.go` 의 `runDeletion` 함수를 아래로 교체한다:

```go
// runDeletion 은 --run 경로다. 다중 선택을 거쳐 선택된 브랜치를 삭제한다.
func runDeletion(c classify.Classified, opts options) int {
	items := make([]pick.Item, len(c.ToDelete))
	byName := map[string]classify.Result{}
	for i, r := range c.ToDelete {
		items[i] = pick.Item{
			Name:         r.Name,
			Signal:       string(r.Signal),
			WorktreePath: r.WorktreePath,
			AgeDays:      r.AgeDays,
			Checked:      r.Signal == classify.SignalGone,
		}
		byName[r.Name] = r
	}
	sel := pick.NewSelection(items)

	var chosen []string
	var ok bool
	switch pick.DetectMode(opts.noTUI) {
	case pick.ModeNone:
		fmt.Fprintln(os.Stderr, "git-tidy: 삭제하려면 터미널이 필요합니다. 목록은 인자 없는 git-tidy 로 확인하세요.")
		return 1
	case pick.ModeTUI:
		var fellBack bool
		chosen, ok, fellBack = pick.RunTUI(sel)
		if fellBack {
			chosen, ok = pick.RunLine(sel, os.Stdin, os.Stdout)
		}
	case pick.ModeLine:
		chosen, ok = pick.RunLine(sel, os.Stdin, os.Stdout)
	}
	if !ok || len(chosen) == 0 {
		fmt.Println("삭제하지 않았습니다.")
		return 0
	}
	return deleteBranches(chosen, byName)
}
```

- [ ] **Step 3: 빌드와 전체 테스트 통과 확인**

Run: `cd apps/git-tidy && go build ./cmd/git-tidy && go test ./...`
Expected: 빌드 성공, 모든 패키지 테스트 PASS.

- [ ] **Step 4: fmt·lint 확인**

Run: `cd apps/git-tidy && gofmt -l . && go vet ./...`
Expected: gofmt 출력 없음(형식 통과), vet 무경고.

- [ ] **Step 5: Commit**

```bash
cd apps/git-tidy && git add cmd/git-tidy/main.go
git commit -m "feat(git-tidy): Result 를 그룹 선택 모델로 배선하고 출력 그룹화

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: 문서 갱신과 1회성 E2E

**Files:**
- Modify: `apps/git-tidy/README.md`
- Modify: `apps/git-tidy/CLAUDE.md`

- [ ] **Step 1: README 동작 설명 갱신**

`apps/git-tidy/README.md` 의 "동작 방식" 4번 항목을, 기본 선택과 그룹 표시를 반영하도록 보강한다. 4번 bullet 을 아래로 교체한다:

```markdown
4. 남은 브랜치가 삭제 대상이다. dry-run 이면 삭제 사유(`gone` → `merged` →
   `stale`)별로 묶어 목록만 출력한다. `--run` 이면 같은 그룹 구조의 선택 화면을
   띄우는데, 가장 확실한 후보인 `[gone]` 만 기본 체크된 상태로 시작하고 사용자가
   나머지를 직접 고른다. `stale` 항목에는 마지막 커밋 기준 경과 일수가, worktree 에
   물린 브랜치에는 worktree 이름이 함께 표시된다.
```

선택 화면 단축키 안내도 "사용" 섹션 아래에 한 줄 덧붙인다(있으면 갱신):

```markdown
선택 화면에서는 `↑↓`/`jk` 이동, `space` 토글(그룹 헤더에서는 그룹 일괄 토글),
`a` 전체 토글, `enter` 삭제, `esc` 취소를 쓴다.
```

- [ ] **Step 2: CLAUDE.md 모델 요약 갱신**

`apps/git-tidy/CLAUDE.md` 의 "정리 모델 요약" 마지막에 한 단락을 덧붙인다:

```markdown
선택 화면은 `pick` 패키지가 담당한다. `classify` 가 신호 순위(gone → merged →
stale)로 정렬한 결과를 받아, `gone` 만 기본 체크한 그룹형 선택 모델을 만든다.
bubbletea 기반 TUI 와 줄 기반 폴백이 같은 모델을 공유하며, 그룹 헤더에서 그룹 일괄
토글을 지원한다. TUI 의 상태 전이(`Update`)는 순수 함수로 분리해 단위 테스트한다.
```

- [ ] **Step 3: 정적 확인**

Run: `cd apps/git-tidy && grep -n "기본 체크\|그룹 일괄 토글\|경과 일수" README.md CLAUDE.md`
Expected: 갱신한 문구가 검색됨(문서 반영 확인).

- [ ] **Step 4: 1회성 E2E (수동, 작성자 직접)**

실제 저장소에서 빌드 후 dry-run 과 선택 화면을 직접 확인한다:

```bash
cd apps/git-tidy && go build -o /tmp/git-tidy-e2e ./cmd/git-tidy
# 임의의 git 저장소로 이동해 dry-run 출력 확인 (그룹 헤더·경과 일수)
( cd <테스트 저장소> && /tmp/git-tidy-e2e --no-fetch )
# 선택 화면 확인 (gone 만 기본 체크, 그룹 토글, 색·worktree 표시)
( cd <테스트 저장소> && /tmp/git-tidy-e2e --run --no-fetch )
```

확인 항목: (1) dry-run 이 gone→merged→stale 순서로 그룹 출력, (2) TUI 에서 `[gone]`만 체크된 채 시작, (3) 헤더에서 space 로 그룹 토글, (4) stale 에 경과 일수·worktree 이름 표시, (5) enter 로 선택 삭제·esc 로 취소. 결과(화면 캡처 또는 출력 텍스트)를 PR 본문에 첨부한다.

- [ ] **Step 5: Commit**

```bash
cd apps/git-tidy && git add README.md CLAUDE.md
git commit -m "docs(git-tidy): 기본 선택·그룹 토글·경과 표시 동작 문서화

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## 완료 후 전체 검토

- spec(`docs/superpowers/specs/2026-06-01-git-tidy-improvements-design.md`)의 각 절을 다시 읽고 구현이 대응하는지 대조한다.
- `cd apps/git-tidy && mise run fmt-check && mise run lint && mise run test && mise run build` 가 모두 통과하는지 확인한다(CI 동일 순서).
- PR 본문은 `@rules/pr-body-template.md` 양식(Overview → Changes → Decision Log → Related)을 따른다.
