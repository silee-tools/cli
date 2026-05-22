# git-tidy Go 재작성 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git-tidy 를 zsh 플러그인에서 Go CLI 바이너리로 재작성하고, 하이브리드 정리 모델(삭제 후보 신호 ∩ 보호 규칙)과 체크박스/줄 기반 다중 선택을 구현한다.

**Architecture:** `git` CLI 를 `os/exec` 로 호출해 브랜치 정보를 모으고(`internal/gitx`), 순수 함수로 삭제 대상을 분류하며(`internal/classify`), 순수 선택 모델 위에 체크박스 TUI 와 줄 기반 선택 두 front-end 를 둔다(`internal/pick`). `cmd/git-tidy` 가 인자 파싱·오케스트레이션·출력을 맡는다.

**Tech Stack:** Go 1.23, `git` CLI, `golang.org/x/term`(터미널 감지·raw 모드), GoReleaser, mise.

설계 근거는 [docs/plans/2026-05-22-git-tidy-cleanup-model.md](2026-05-22-git-tidy-cleanup-model.md) 를 단일 기준으로 삼는다.

---

## File Structure

```
apps/git-tidy/
  go.mod                              module github.com/silee-tools/git-tidy
  .mise.toml                          Go 태스크(build/test/lint/fmt/install)
  .goreleaser.yaml                    실제 Go 빌드
  cmd/git-tidy/main.go                인자 파싱·디스패치·오케스트레이션·출력
  cmd/git-tidy/main_test.go
  internal/gitx/gitx.go               git CLI 래퍼
  internal/gitx/gitx_test.go          순수 파서 단위 테스트
  internal/classify/classify.go       Branch 타입 + Classify(하이브리드 모델)
  internal/classify/classify_test.go
  internal/pick/model.go              순수 선택 모델(체크 상태·토글)
  internal/pick/model_test.go
  internal/pick/pick.go               방식 자동 선택(IsTerminal·TERM)
  internal/pick/line.go               줄 기반 선택
  internal/pick/tui.go                체크박스 TUI(raw 모드)
  completions/_git-tidy               zsh 자동완성
  completions/git-tidy.bash           bash 자동완성
  README.md  CLAUDE.md  PRD.md  CHANGELOG.md
```

기존 `git-tidy.plugin.zsh` 는 Task 1 에서 삭제한다(git 히스토리에 남는다).

---

## Task 1: Go 모듈 스캐폴드 + 디렉터리 전환

**Files:**
- Create: `apps/git-tidy/go.mod`, `apps/git-tidy/cmd/git-tidy/main.go`
- Modify: `apps/git-tidy/.mise.toml`, `apps/git-tidy/.goreleaser.yaml`, `.github/workflows/git-tidy-ci.yml`
- Delete: `apps/git-tidy/git-tidy.plugin.zsh`

- [ ] **Step 1: go.mod 생성**

`apps/git-tidy/go.mod`:
```
module github.com/silee-tools/git-tidy

go 1.23
```

- [ ] **Step 2: 최소 컴파일 가능한 main.go 생성**

`apps/git-tidy/cmd/git-tidy/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

var version = "dev"

// versionLine 은 모노레포 전 도구가 공유하는 표준 버전 한 줄을 만든다.
func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "-v", "--version":
			fmt.Fprint(os.Stdout, versionLine("git-tidy", version))
			return
		case "-h", "--help":
			fmt.Print(helpText)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "git-tidy: not implemented yet")
}

const helpText = `Usage: git-tidy [--run] [options]

Clean up local git branches that are done or stale.
`
```

- [ ] **Step 3: .mise.toml 을 Go 태스크로 교체**

`apps/git-tidy/.mise.toml` 전체를 다음으로 바꾼다:
```toml
[tools]
go = "1.23"
"aqua:golangci/golangci-lint" = "latest"

[tasks.build]
description = "Build the project"
run = "go build ./cmd/git-tidy"

[tasks.test]
description = "Run tests"
run = "go test ./..."

[tasks.lint]
description = "Run linter"
run = "golangci-lint run"

[tasks.fmt]
description = "Format code"
run = "gofmt -w ."

[tasks.fmt-check]
description = "Check formatting (CI)"
run = "test -z \"$(gofmt -l .)\""

[tasks.install]
description = "Build and install to ~/.local/bin"
run = "go build -o ~/.local/bin/git-tidy ./cmd/git-tidy"

[tasks.uninstall]
description = "Remove local dev build"
run = "rm -f ~/.local/bin/git-tidy"
```

- [ ] **Step 4: .goreleaser.yaml 을 실제 Go 빌드로 교체**

`apps/git-tidy/.goreleaser.yaml` 전체를 다음으로 바꾼다:
```yaml
version: 2

project_name: git-tidy

builds:
  - main: ./cmd/git-tidy
    binary: git-tidy
    ldflags:
      - -s -w -X main.version={{.Version}}
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64

archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}-v{{ .Version }}-{{ .Os }}-{{ .Arch }}"
    files:
      - completions/*

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^chore:"

release:
  disable: true
```

- [ ] **Step 5: CI 워크플로를 Go 기준으로 교체**

`.github/workflows/git-tidy-ci.yml` 의 `jobs.test.steps` 를 jg-ci.yml 과 동일한 형태로 바꾼다(zsh 설치 step 제거):
```yaml
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
        with:
          working_directory: apps/git-tidy
      - run: mise run fmt-check
      - run: mise run lint
      - run: mise run test
      - run: mise run build
```
`on.push.paths` 와 `on.pull_request.paths` 는 그대로 둔다.

- [ ] **Step 6: 옛 zsh 플러그인 삭제**

```bash
git rm apps/git-tidy/git-tidy.plugin.zsh
```

- [ ] **Step 7: 빌드·포맷 확인**

Run: `cd apps/git-tidy && mise run fmt-check && mise run build && ./git-tidy -v`
Expected: 포맷 통과, 빌드 성공, `git-tidy vdev © 2026 silee-tools` 출력.

- [ ] **Step 8: Commit**

```bash
git add apps/git-tidy/ .github/workflows/git-tidy-ci.yml
git commit -m "refactor(git-tidy)!: scaffold Go module, drop zsh plugin"
```

---

## Task 2: git CLI 래퍼 (`internal/gitx`)

**Files:**
- Create: `apps/git-tidy/internal/gitx/gitx.go`, `apps/git-tidy/internal/gitx/gitx_test.go`

- [ ] **Step 1: porcelain 파서 실패 테스트 작성**

`apps/git-tidy/internal/gitx/gitx_test.go`:
```go
package gitx

import (
	"reflect"
	"testing"
)

func TestParseBranchLines(t *testing.T) {
	// for-each-ref --format='%(refname:short)\x00%(upstream:track)\x00%(committerdate:unix)'
	input := "feature-a\x00[gone]\x001700000000\n" +
		"feature-b\x00\x001800000000\n" +
		"feature-c\x00[ahead 1]\x001900000000\n"
	got := parseBranchLines(input)
	want := []BranchRef{
		{Name: "feature-a", UpstreamGone: true, HasUpstream: true, CommitUnix: 1700000000},
		{Name: "feature-b", UpstreamGone: false, HasUpstream: false, CommitUnix: 1800000000},
		{Name: "feature-c", UpstreamGone: false, HasUpstream: true, CommitUnix: 1900000000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranchLines mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestParseWorktreeBranches(t *testing.T) {
	input := "worktree /a\nbranch refs/heads/main\n\nworktree /b\nbranch refs/heads/feature-x\n\nworktree /c\ndetached\n"
	got := parseWorktreeBranches(input)
	want := map[string]string{"main": "/a", "feature-x": "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktreeBranches mismatch\n got=%+v\nwant=%+v", got, want)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-tidy && go test ./internal/gitx/`
Expected: FAIL — `parseBranchLines`/`BranchRef`/`parseWorktreeBranches` 미정의.

- [ ] **Step 3: gitx.go 구현**

`apps/git-tidy/internal/gitx/gitx.go`:
```go
// Package gitx 는 git CLI 를 호출해 git-tidy 가 쓰는 정보를 모은다.
package gitx

import (
	"os/exec"
	"strconv"
	"strings"
)

// BranchRef 는 로컬 브랜치 하나의 git 메타데이터다.
type BranchRef struct {
	Name         string
	HasUpstream  bool // upstream 추적 브랜치가 설정돼 있는가
	UpstreamGone bool // upstream 이 [gone] 인가
	CommitUnix   int64
}

// run 은 git 을 실행하고 표준 출력을 돌려준다.
func run(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return string(out), err
}

// IsRepo 는 현재 디렉터리가 git 저장소인지 본다.
func IsRepo() bool {
	_, err := run("rev-parse", "--git-dir")
	return err == nil
}

// FetchPrune 는 git fetch --prune 를 실행한다. 실패는 무시한다(오프라인 등).
func FetchPrune() {
	_, _ = run("fetch", "--prune")
}

// CurrentBranch 는 체크아웃된 브랜치 이름을 돌려준다(detached 면 빈 문자열).
func CurrentBranch() string {
	out, _ := run("branch", "--show-current")
	return strings.TrimSpace(out)
}

// LocalBranches 는 모든 로컬 브랜치의 메타데이터를 돌려준다.
func LocalBranches() ([]BranchRef, error) {
	out, err := run("for-each-ref",
		"--format=%(refname:short)%00%(upstream:track)%00%(committerdate:unix)",
		"refs/heads")
	if err != nil {
		return nil, err
	}
	return parseBranchLines(out), nil
}

func parseBranchLines(out string) []BranchRef {
	var refs []BranchRef
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 3 {
			continue
		}
		unix, _ := strconv.ParseInt(f[2], 10, 64)
		track := f[1]
		refs = append(refs, BranchRef{
			Name:         f[0],
			HasUpstream:  track != "",
			UpstreamGone: track == "[gone]",
			CommitUnix:   unix,
		})
	}
	return refs
}

// WorktreeBranches 는 worktree 에 체크아웃된 브랜치 → worktree 경로 맵을 돌려준다.
func WorktreeBranches() (map[string]string, error) {
	out, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeBranches(out), nil
}

func parseWorktreeBranches(out string) map[string]string {
	result := map[string]string{}
	var path string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			result[strings.TrimPrefix(line, "branch refs/heads/")] = path
		}
	}
	return result
}

// BaseBranch 는 main/master/trunk 등 기본 브랜치를 자동 감지한다.
func BaseBranch() string {
	for _, name := range []string{"main", "master", "trunk"} {
		if _, err := run("show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	return "main"
}

// MergedBranches 는 base 에 머지된 로컬 브랜치 이름 집합을 돌려준다.
func MergedBranches(base string) (map[string]bool, error) {
	out, err := run("branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

// MergeBaseUnix 는 base 와 branch 의 분기점 커밋 시각(unix)을 돌려준다.
func MergeBaseUnix(base, branch string) (int64, bool) {
	mb, err := run("merge-base", base, branch)
	if err != nil {
		return 0, false
	}
	out, err := run("show", "-s", "--format=%ct", strings.TrimSpace(mb))
	if err != nil {
		return 0, false
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return unix, err == nil
}

// DeleteBranch 는 git branch -D 로 브랜치를 삭제한다.
func DeleteBranch(name string) error {
	_, err := run("branch", "-D", name)
	return err
}

// RemoveWorktree 는 worktree 를 제거한다. 미커밋 변경이 있으면 git 이 거부한다.
func RemoveWorktree(path string) error {
	_, err := run("worktree", "remove", path)
	return err
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/git-tidy && go test ./internal/gitx/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/git-tidy/internal/gitx/
git commit -m "feat(git-tidy): add git CLI wrapper"
```

---

## Task 3: 하이브리드 분류 (`internal/classify`)

**Files:**
- Create: `apps/git-tidy/internal/classify/classify.go`, `apps/git-tidy/internal/classify/classify_test.go`

- [ ] **Step 1: 분류 실패 테스트 작성**

`apps/git-tidy/internal/classify/classify_test.go`:
```go
package classify

import (
	"reflect"
	"testing"

	"github.com/silee-tools/git-tidy/internal/gitx"
)

func TestClassify(t *testing.T) {
	now := int64(1_000_000_000)
	staleDays := 20
	day := int64(86400)
	old := now - 30*day  // stale 창 밖
	recent := now - 5*day // stale 창 안

	in := Input{
		Now:       now,
		StaleDays: staleDays,
		Base:      "main",
		Current:   "feature-current",
		Branches: []gitx.BranchRef{
			{Name: "feature-current", CommitUnix: old},          // 보호: 현재 브랜치
			{Name: "main", CommitUnix: old},                     // 보호: base
			{Name: "feature-gone", UpstreamGone: true, HasUpstream: true, CommitUnix: recent}, // 후보: gone
			{Name: "feature-merged", HasUpstream: true, CommitUnix: recent},                   // 후보: merged
			{Name: "feature-stale", HasUpstream: true, CommitUnix: old},                       // 후보: stale(마지막 커밋)
			{Name: "feature-active", HasUpstream: true, CommitUnix: recent},                   // 후보 아님
		},
		Merged:           map[string]bool{"feature-merged": true, "main": true},
		Worktrees:        map[string]string{},
		MergeBaseUnix:    func(branch string) (int64, bool) { return recent, true }, // 분기점은 전부 최근
	}

	got := Classify(in)

	wantDelete := []Result{
		{Name: "feature-gone", Signal: SignalGone},
		{Name: "feature-merged", Signal: SignalMerged},
		{Name: "feature-stale", Signal: SignalStale},
	}
	if !reflect.DeepEqual(got.ToDelete, wantDelete) {
		t.Errorf("ToDelete mismatch\n got=%+v\nwant=%+v", got.ToDelete, wantDelete)
	}
	if got.OtherCount != 1 { // feature-active
		t.Errorf("OtherCount = %d, want 1", got.OtherCount)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-tidy && go test ./internal/classify/`
Expected: FAIL — `Input`/`Classify`/`Result`/`Signal*` 미정의.

- [ ] **Step 3: classify.go 구현**

`apps/git-tidy/internal/classify/classify.go`:
```go
// Package classify 는 하이브리드 모델로 로컬 브랜치를 삭제 대상과 그 외로 나눈다.
package classify

import "github.com/silee-tools/git-tidy/internal/gitx"

// Signal 은 브랜치가 삭제 후보가 된 이유다.
type Signal string

const (
	SignalGone   Signal = "gone"
	SignalMerged Signal = "merged"
	SignalStale  Signal = "stale"
)

// Input 은 분류에 필요한 모든 데이터다. git 호출과 분리해 순수 함수로 둔다.
type Input struct {
	Now           int64
	StaleDays     int
	Base          string
	Current       string
	Branches      []gitx.BranchRef
	Merged        map[string]bool          // base 에 머지된 브랜치
	Worktrees     map[string]string        // 브랜치 → worktree 경로
	MergeBaseUnix func(branch string) (int64, bool)
}

// Result 는 삭제 대상 브랜치 하나다.
type Result struct {
	Name         string
	Signal       Signal
	WorktreePath string // worktree 에 물려 있으면 그 경로, 아니면 빈 문자열
}

// Classified 는 분류 결과다.
type Classified struct {
	ToDelete   []Result
	OtherCount int // 후보도 아니고 보호도 아닌 평범한 브랜치 수
}

// Classify 는 Input 을 받아 삭제 대상을 가려낸다.
func Classify(in Input) Classified {
	var out Classified
	cutoff := in.Now - int64(in.StaleDays)*86400
	for _, b := range in.Branches {
		// 보호 규칙: 현재 브랜치, base 브랜치는 절대 삭제하지 않는다.
		if b.Name == in.Current || b.Name == in.Base {
			continue
		}
		sig, isCandidate := candidateSignal(b, in, cutoff)
		if !isCandidate {
			out.OtherCount++
			continue
		}
		out.ToDelete = append(out.ToDelete, Result{
			Name:         b.Name,
			Signal:       sig,
			WorktreePath: in.Worktrees[b.Name],
		})
	}
	return out
}

// candidateSignal 은 브랜치의 첫 삭제 후보 신호를 돌려준다.
func candidateSignal(b gitx.BranchRef, in Input, cutoff int64) (Signal, bool) {
	if b.UpstreamGone {
		return SignalGone, true
	}
	if in.Merged[b.Name] {
		return SignalMerged, true
	}
	if isStale(b, in, cutoff) {
		return SignalStale, true
	}
	return "", false
}

// isStale 은 마지막 커밋 또는 분기점이 stale 창보다 오래됐는지 본다(OR).
func isStale(b gitx.BranchRef, in Input, cutoff int64) bool {
	if b.CommitUnix > 0 && b.CommitUnix < cutoff {
		return true
	}
	if mb, ok := in.MergeBaseUnix(b.Name); ok && mb < cutoff {
		return true
	}
	return false
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/git-tidy && go test ./internal/classify/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/git-tidy/internal/classify/
git commit -m "feat(git-tidy): add hybrid branch classification"
```

---

## Task 4: 순수 선택 모델 (`internal/pick/model.go`)

**Files:**
- Create: `apps/git-tidy/internal/pick/model.go`, `apps/git-tidy/internal/pick/model_test.go`

- [ ] **Step 1: 선택 모델 실패 테스트 작성**

`apps/git-tidy/internal/pick/model_test.go`:
```go
package pick

import (
	"reflect"
	"testing"
)

func TestSelectionModel(t *testing.T) {
	m := NewSelection([]string{"a", "b", "c"})

	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("초기 상태는 전부 체크여야 함, got %v", got)
	}

	m.Toggle(1) // b 해제
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("Toggle(1) 후 %v, want [a c]", got)
	}

	m.ToggleAll() // 하나라도 켜져 있으면 전체 해제
	if got := m.Checked(); len(got) != 0 {
		t.Errorf("ToggleAll 후 전체 해제 기대, got %v", got)
	}

	m.ToggleAll() // 전부 꺼져 있으면 전체 체크
	if got := m.Checked(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("ToggleAll 재호출 후 전체 체크 기대, got %v", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-tidy && go test ./internal/pick/`
Expected: FAIL — `NewSelection`/`Selection` 미정의.

- [ ] **Step 3: model.go 구현**

`apps/git-tidy/internal/pick/model.go`:
```go
// Package pick 는 삭제 대상 다중 선택을 담당한다. 순수 선택 모델 위에
// 체크박스 TUI 와 줄 기반 선택 두 front-end 를 둔다.
package pick

// Selection 은 항목별 체크 상태를 들고 있는 순수 모델이다.
// 렌더링·입력과 분리돼 있어 TUI 와 줄 기반 모드가 함께 쓰고 단위 테스트한다.
type Selection struct {
	items   []string
	checked []bool
}

// NewSelection 은 모든 항목이 체크된 상태의 모델을 만든다.
func NewSelection(items []string) *Selection {
	checked := make([]bool, len(items))
	for i := range checked {
		checked[i] = true
	}
	return &Selection{items: items, checked: checked}
}

// Items 는 전체 항목을 돌려준다.
func (s *Selection) Items() []string { return s.items }

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

// Checked 는 체크된 항목들을 순서대로 돌려준다.
func (s *Selection) Checked() []string {
	var out []string
	for i, c := range s.checked {
		if c {
			out = append(out, s.items[i])
		}
	}
	return out
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/git-tidy && go test ./internal/pick/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/git-tidy/internal/pick/model.go apps/git-tidy/internal/pick/model_test.go
git commit -m "feat(git-tidy): add pure multi-select model"
```

---

## Task 5: 방식 자동 선택 + 줄 기반 선택 (`internal/pick`)

**Files:**
- Create: `apps/git-tidy/internal/pick/pick.go`, `apps/git-tidy/internal/pick/line.go`

- [ ] **Step 1: 방식 자동 선택 — `pick.go`**

`apps/git-tidy/internal/pick/pick.go`:
```go
package pick

import (
	"os"

	"golang.org/x/term"
)

// Mode 는 다중 선택 방식이다.
type Mode int

const (
	ModeTUI  Mode = iota // 체크박스 TUI
	ModeLine             // 줄 기반 선택
	ModeNone             // 입력 불가(터미널 아님)
)

// DetectMode 는 실행 환경을 보고 선택 방식을 고른다.
// forceLine 이 참이면 터미널인 한 항상 ModeLine 을 돌려준다.
func DetectMode(forceLine bool) Mode {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ModeNone
	}
	if forceLine {
		return ModeLine
	}
	if t := os.Getenv("TERM"); t == "" || t == "dumb" {
		return ModeLine
	}
	return ModeTUI
}
```

- [ ] **Step 2: x/term 의존 추가**

Run: `cd apps/git-tidy && go get golang.org/x/term && go mod tidy`
Expected: `go.mod`·`go.sum` 에 `golang.org/x/term` 추가.

- [ ] **Step 3: 줄 기반 선택 — `line.go`**

`apps/git-tidy/internal/pick/line.go`:
```go
package pick

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RunLine 은 줄 입력으로 다중 선택을 진행하고 체크된 항목을 돌려준다.
// 두 번째 반환값이 false 면 사용자가 취소한 것이다.
// labels 는 항목마다 옆에 붙일 부가 설명(신호 등)이다. in/out 은 테스트를
// 위해 주입한다.
func RunLine(sel *Selection, labels []string, in io.Reader, out io.Writer) ([]string, bool) {
	r := bufio.NewScanner(in)
	for {
		renderLine(sel, labels, out)
		fmt.Fprint(out, "번호=토글, a=전체토글, 빈 줄=완료, q=취소 > ")
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

func renderLine(sel *Selection, labels []string, out io.Writer) {
	for i, item := range sel.Items() {
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		fmt.Fprintf(out, "  %2d. [%s] %s  %s\n", i+1, mark, item, labels[i])
	}
}

func confirmLine(sel *Selection, r *bufio.Scanner, out io.Writer) ([]string, bool) {
	checked := sel.Checked()
	if len(checked) == 0 {
		return nil, true
	}
	fmt.Fprintf(out, "%d개 브랜치를 삭제합니다. 진행할까요? [y/N] ", len(checked))
	if !r.Scan() {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(r.Text()), "y") {
		return checked, true
	}
	return nil, false
}
```

- [ ] **Step 4: 줄 기반 선택 테스트 작성**

`apps/git-tidy/internal/pick/line_test.go`:
```go
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
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `cd apps/git-tidy && go test ./internal/pick/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/git-tidy/internal/pick/ apps/git-tidy/go.mod apps/git-tidy/go.sum
git commit -m "feat(git-tidy): add mode detection and line-based selection"
```

---

## Task 6: 체크박스 TUI (`internal/pick/tui.go`)

**Files:**
- Create: `apps/git-tidy/internal/pick/tui.go`

raw 모드 입출력은 단위 테스트가 어렵다. 선택 로직은 Task 4 의 `Selection` 모델이 이미 검증하므로, `tui.go` 는 그 모델을 키 입력과 화면에 연결하는 얇은 층으로만 둔다. 검증은 Task 11 의 1회성 E2E 가 맡는다.

- [ ] **Step 1: tui.go 구현**

`apps/git-tidy/internal/pick/tui.go`:
```go
package pick

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// RunTUI 는 raw 모드 체크박스 목록으로 다중 선택을 진행한다.
// 두 번째 반환값이 false 면 취소다. raw 모드 진입에 실패하면 ok=false,
// 세 번째 반환값(fellBack)이 true 가 되어 호출자가 줄 기반으로 폴백한다.
func RunTUI(sel *Selection, labels []string) (checked []string, ok bool, fellBack bool) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, false, true
	}
	defer term.Restore(fd, oldState)

	cursor := 0
	buf := make([]byte, 3)
	for {
		renderTUI(sel, labels, cursor)
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			continue
		}
		switch {
		case buf[0] == 3 || buf[0] == 27 && n == 1: // Ctrl-C / ESC
			clearTUI(len(sel.Items()))
			return nil, false, false
		case buf[0] == 13: // Enter
			clearTUI(len(sel.Items()))
			return sel.Checked(), true, false
		case buf[0] == ' ':
			sel.Toggle(cursor)
		case buf[0] == 'a':
			sel.ToggleAll()
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 65: // ↑
			if cursor > 0 {
				cursor--
			}
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 66: // ↓
			if cursor < len(sel.Items())-1 {
				cursor++
			}
		}
	}
}

func renderTUI(sel *Selection, labels []string, cursor int) {
	clearTUI(len(sel.Items()))
	for i, item := range sel.Items() {
		mark := " "
		if sel.IsChecked(i) {
			mark = "x"
		}
		pointer := "  "
		if i == cursor {
			pointer = "> "
		}
		fmt.Printf("%s[%s] %s  %s\r\n", pointer, mark, item, labels[i])
	}
	fmt.Print("스페이스=토글, a=전체토글, Enter=확정, ESC=취소\r\n")
}

// clearTUI 는 직전 렌더링(목록 줄 + 안내 1줄)을 지워 다시 그릴 자리를 만든다.
func clearTUI(itemCount int) {
	for i := 0; i < itemCount+1; i++ {
		fmt.Print("\033[1A\033[2K")
	}
}
```

- [ ] **Step 2: 빌드 확인**

Run: `cd apps/git-tidy && go build ./... && go test ./...`
Expected: 빌드 성공, 기존 테스트 PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/git-tidy/internal/pick/tui.go
git commit -m "feat(git-tidy): add raw-mode checkbox TUI"
```

---

## Task 7: main.go — 인자 파싱·오케스트레이션·출력

**Files:**
- Modify: `apps/git-tidy/cmd/git-tidy/main.go`
- Create: `apps/git-tidy/cmd/git-tidy/main_test.go`

- [ ] **Step 1: 인자 파싱 실패 테스트 작성**

`apps/git-tidy/cmd/git-tidy/main_test.go`:
```go
package main

import "testing"

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want options
	}{
		{[]string{}, options{staleDays: 20}},
		{[]string{"--run"}, options{run: true, staleDays: 20}},
		{[]string{"--run", "--no-tui"}, options{run: true, noTUI: true, staleDays: 20}},
		{[]string{"--stale-days=7"}, options{staleDays: 7}},
		{[]string{"--no-fetch"}, options{noFetch: true, staleDays: 20}},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args)
		if err != nil {
			t.Errorf("parseArgs(%v) error: %v", c.args, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseArgs(%v) = %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestParseArgsRejectsUnknown(t *testing.T) {
	if _, err := parseArgs([]string{"--bogus"}); err == nil {
		t.Error("알 수 없는 플래그는 오류여야 함")
	}
}

func TestVersionLine(t *testing.T) {
	want := "git-tidy v1.2.3 © 2026 silee-tools\n"
	if got := versionLine("git-tidy", "1.2.3"); got != want {
		t.Errorf("versionLine = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-tidy && go test ./cmd/git-tidy/`
Expected: FAIL — `options`/`parseArgs` 미정의.

- [ ] **Step 3: main.go 구현**

`apps/git-tidy/cmd/git-tidy/main.go` 전체를 다음으로 바꾼다. `GIT_TIDY_STALE_DAYS` 환경변수는 기본값으로, `--stale-days` 플래그는 그 위에 우선한다.
```go
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/silee-tools/git-tidy/internal/classify"
	"github.com/silee-tools/git-tidy/internal/gitx"
	"github.com/silee-tools/git-tidy/internal/pick"
)

var version = "dev"

func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

type options struct {
	run       bool
	noTUI     bool
	noFetch   bool
	staleDays int
}

func defaultStaleDays() int {
	if v := os.Getenv("GIT_TIDY_STALE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 20
}

// parseArgs 는 인자를 옵션으로 바꾼다. -v/-h 는 상위에서 먼저 처리한다.
func parseArgs(args []string) (options, error) {
	o := options{staleDays: defaultStaleDays()}
	for _, a := range args {
		switch {
		case a == "--run":
			o.run = true
		case a == "--no-tui":
			o.noTUI = true
		case a == "--no-fetch":
			o.noFetch = true
		case len(a) > 13 && a[:13] == "--stale-days=":
			n, err := strconv.Atoi(a[13:])
			if err != nil || n <= 0 {
				return o, fmt.Errorf("잘못된 --stale-days 값: %s", a[13:])
			}
			o.staleDays = n
		default:
			return o, fmt.Errorf("알 수 없는 옵션: %s", a)
		}
	}
	return o, nil
}

const helpText = `Usage: git-tidy [--run] [options]

작업이 끝났거나 오래 방치된 로컬 git 브랜치를 정리한다.

  git-tidy              dry-run — 삭제 대상만 표시
  git-tidy --run        삭제 대상을 다중 선택해 삭제
  git-tidy --run --no-tui  체크박스 TUI 대신 줄 기반 선택
  --stale-days=N        stale 판정 창 (기본 20, GIT_TIDY_STALE_DAYS)
  --no-fetch            git fetch --prune 건너뛰기
  -v, --version         버전 출력
  -h, --help            도움말 출력
`

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "-v", "--version":
			fmt.Fprint(os.Stdout, versionLine("git-tidy", version))
			return
		case "-h", "--help":
			fmt.Print(helpText)
			return
		}
	}

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-tidy:", err)
		fmt.Fprintln(os.Stderr, "git-tidy --help 로 사용법을 확인하세요.")
		os.Exit(1)
	}
	os.Exit(run(opts))
}

// run 은 git-tidy 본체다. 종료 코드를 돌려준다.
func run(opts options) int {
	if !gitx.IsRepo() {
		fmt.Fprintln(os.Stderr, "git-tidy: git 저장소가 아닙니다.")
		return 1
	}
	if !opts.noFetch {
		gitx.FetchPrune()
	}

	result, err := buildClassification(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-tidy:", err)
		return 1
	}

	if len(result.ToDelete) == 0 {
		fmt.Println("정리할 브랜치가 없습니다.")
		return 0
	}
	printTargets(result)

	if !opts.run {
		fmt.Println("\n→ git-tidy --run 으로 삭제를 진행하세요.")
		return 0
	}
	return runDeletion(result, opts)
}

func buildClassification(opts options) (classify.Classified, error) {
	branches, err := gitx.LocalBranches()
	if err != nil {
		return classify.Classified{}, err
	}
	base := gitx.BaseBranch()
	merged, err := gitx.MergedBranches(base)
	if err != nil {
		return classify.Classified{}, err
	}
	worktrees, err := gitx.WorktreeBranches()
	if err != nil {
		return classify.Classified{}, err
	}
	in := classify.Input{
		Now:       time.Now().Unix(),
		StaleDays: opts.staleDays,
		Base:      base,
		Current:   gitx.CurrentBranch(),
		Branches:  branches,
		Merged:    merged,
		Worktrees: worktrees,
		MergeBaseUnix: func(branch string) (int64, bool) {
			return gitx.MergeBaseUnix(base, branch)
		},
	}
	return classify.Classify(in), nil
}

func printTargets(c classify.Classified) {
	fmt.Printf("삭제 대상 (%d):\n", len(c.ToDelete))
	for _, r := range c.ToDelete {
		line := fmt.Sprintf("  %s  (%s)", r.Name, r.Signal)
		if r.WorktreePath != "" {
			line += "  [worktree 동반 제거]"
		}
		fmt.Println(line)
	}
	if c.OtherCount > 0 {
		fmt.Printf("그 외 브랜치 %d개는 정리 대상이 아닙니다.\n", c.OtherCount)
	}
}

// runDeletion 은 --run 경로다. 다중 선택을 거쳐 선택된 브랜치를 삭제한다.
func runDeletion(c classify.Classified, opts options) int {
	names := make([]string, len(c.ToDelete))
	labels := make([]string, len(c.ToDelete))
	byName := map[string]classify.Result{}
	for i, r := range c.ToDelete {
		names[i] = r.Name
		labels[i] = "(" + string(r.Signal) + ")"
		byName[r.Name] = r
	}
	sel := pick.NewSelection(names)

	var chosen []string
	var ok bool
	switch pick.DetectMode(opts.noTUI) {
	case pick.ModeNone:
		fmt.Fprintln(os.Stderr, "git-tidy: 삭제하려면 터미널이 필요합니다. 목록은 인자 없는 git-tidy 로 확인하세요.")
		return 1
	case pick.ModeTUI:
		var fellBack bool
		chosen, ok, fellBack = pick.RunTUI(sel, labels)
		if fellBack {
			chosen, ok = pick.RunLine(sel, labels, os.Stdin, os.Stdout)
		}
	case pick.ModeLine:
		chosen, ok = pick.RunLine(sel, labels, os.Stdin, os.Stdout)
	}
	if !ok || len(chosen) == 0 {
		fmt.Println("삭제하지 않았습니다.")
		return 0
	}
	return deleteBranches(chosen, byName)
}

func deleteBranches(chosen []string, byName map[string]classify.Result) int {
	failed := 0
	for _, name := range chosen {
		r := byName[name]
		if r.WorktreePath != "" {
			if err := gitx.RemoveWorktree(r.WorktreePath); err != nil {
				fmt.Printf("  실패: %s (worktree 정리 안 됨)\n", name)
				failed++
				continue
			}
		}
		if err := gitx.DeleteBranch(name); err != nil {
			fmt.Printf("  실패: %s\n", name)
			failed++
			continue
		}
		fmt.Printf("  삭제됨: %s\n", name)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: 테스트·빌드 통과 확인**

Run: `cd apps/git-tidy && go test ./... && mise run lint && mise run fmt-check && mise run build`
Expected: 전부 PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/git-tidy/cmd/git-tidy/
git commit -m "feat(git-tidy): wire arg parsing, classification, deletion"
```

---

## Task 8: 자동완성

**Files:**
- Create: `apps/git-tidy/completions/_git-tidy`, `apps/git-tidy/completions/git-tidy.bash`

- [ ] **Step 1: zsh 자동완성 작성**

`apps/git-tidy/completions/_git-tidy`:
```
#compdef git-tidy

_arguments \
  '--run[삭제 대상을 다중 선택해 삭제]' \
  '--no-tui[체크박스 TUI 대신 줄 기반 선택]' \
  '--stale-days=[stale 판정 창 (일)]:days:(7 14 20 30)' \
  '--no-fetch[git fetch --prune 건너뛰기]' \
  '--version[버전 출력]' \
  '--help[도움말 출력]'
```

- [ ] **Step 2: bash 자동완성 작성**

`apps/git-tidy/completions/git-tidy.bash`:
```bash
_git_tidy() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--run --no-tui --stale-days= --no-fetch --version --help"
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_tidy git-tidy
```

- [ ] **Step 3: Commit**

```bash
git add apps/git-tidy/completions/
git commit -m "feat(git-tidy): add zsh and bash completions"
```

---

## Task 9: 저장소 통합 — 문서·formula

**Files:**
- Modify: `CLAUDE.md`, `apps/git-tidy/PRD.md`, `apps/git-tidy/README.md`, `apps/git-tidy/CLAUDE.md`, `docs/plans/2026-05-21-roadmap.md`
- Modify (별도 저장소): `homebrew-tap/Formula/git-tidy.rb`

- [ ] **Step 1: 루트 CLAUDE.md 갱신**

`CLAUDE.md` 의 "모노레포 컨벤션" 에서 git-tidy 를 순수 zsh 플러그인으로 설명한 문장을 삭제한다. git-tidy 가 이제 jg·totp 와 같은 일반 Go 도구임을 반영하고, 버전 conformance 게이트의 zsh 플러그인 분기 설명에서 git-tidy 예시를 뺀다.

- [ ] **Step 2: apps/git-tidy/PRD.md 갱신**

설계 문서 [2026-05-22-git-tidy-cleanup-model.md](2026-05-22-git-tidy-cleanup-model.md) 의 "PRD 갱신 사항" 절대로 PRD 를 고친다: 한 줄 정의를 "명령줄 도구" 로, 정리 모델을 하이브리드로, 기능 범위를 새 플래그 집합으로, 외부 의존에서 zsh 필수 의존 제거, 품질 dimension 표에서 `test_quality` 를 opt-in 으로, 셸 통합 관련 dimension 의 reason 을 "셸 상태를 바꾸지 않는 순수 CLI" 로 다시 쓴다.

- [ ] **Step 3: apps/git-tidy/README.md·CLAUDE.md 갱신**

`apps/git-tidy/README.md` 의 설치·사용법을 Go 바이너리 기준으로 다시 쓴다(brew 설치, `git-tidy`/`--run`/`--no-tui`/`--stale-days`). `apps/git-tidy/CLAUDE.md` 의 "순수 zsh 플러그인" 설명을 Go 도구 구조 설명으로 바꾼다.

- [ ] **Step 4: roadmap 갱신**

`docs/plans/2026-05-21-roadmap.md` 단계 4 의 git-tidy 후속 작업 서술을 "Go 재작성" 으로 고친다. "zsh 테스트(zunit) 도입" 항목과 리스크 3 의 zunit 언급을 제거한다.

- [ ] **Step 5: homebrew-tap formula 갱신**

`homebrew-tap/Formula/git-tidy.rb` 를 바이너리 설치 방식으로 바꾼다(jg.rb 를 템플릿으로, url 파일명을 `git-tidy-v<버전>-<os>-<arch>.tar.gz` 형식으로). sha256·version 은 release-please 후속 step 이 자동 갱신하므로 placeholder 로 둔다.

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md apps/git-tidy/PRD.md apps/git-tidy/README.md apps/git-tidy/CLAUDE.md docs/plans/2026-05-21-roadmap.md
git commit -m "docs(git-tidy): update repo docs and PRD for Go rewrite"
```

homebrew-tap 변경은 그 저장소에서 따로 commit 한다.

---

## Task 10: 전체 검증

- [ ] **Step 1: 전체 테스트·lint·빌드**

Run: `cd apps/git-tidy && go test -count=1 ./... && mise run lint && mise run fmt-check && mise run build`
Expected: 전부 PASS.

- [ ] **Step 2: 버전 conformance 확인**

Run: 저장소 루트에서 `bash scripts/check-version-format.sh` (git-tidy 가 Go 경로로 라우팅돼 통과하는지).
Expected: git-tidy `-v`/`--version` 표준 형식 통과.

---

## Task 11: 1회성 E2E (TDD 강화 조항 2)

raw 모드 TUI 와 git 실제 호출은 단위 테스트 경계 밖이다. 임시 git 저장소를 만들어 변경 경로를 직접 검증한다.

- [ ] **Step 1: 임시 저장소로 dry-run 검증**

임시 git repo 에 `[gone]`·merged·stale·정상 브랜치를 각각 만들고 `git-tidy` 를 실행해, 삭제 대상에 앞 셋만 알맞은 신호와 함께 나오고 정상 브랜치는 "그 외" 로 집계되는지 확인한다. 셸 캡처를 보관한다.

- [ ] **Step 2: `--run` 체크박스 TUI 검증**

같은 저장소에서 `git-tidy --run` 을 실제 터미널에서 실행해, 전부 체크된 상태로 떠서 하나를 해제하고 Enter 로 확정하면 해제한 것만 남고 나머지가 삭제되는지 확인한다. 화면 캡처를 보관한다.

- [ ] **Step 3: `--run --no-tui` 줄 기반 검증**

`git-tidy --run --no-tui` 로 줄 기반 선택이 뜨고, 번호 토글·완료·`[y/N]` 확정이 동작하는지 확인한다. 셸 캡처를 보관한다.

- [ ] **Step 4: worktree 동반 제거 검증**

worktree 에 체크아웃된 후보 브랜치를 만들어 `--run` 으로 삭제할 때 worktree 가 함께 제거되는지, 미커밋 변경이 있으면 "삭제 실패(worktree 정리 안 됨)" 로 보고되는지 확인한다.

- [ ] **Step 5: 증거 보관**

각 단계의 캡처를 PR 본문 검증 절에 첨부할 수 있게 보관한다.

---

## 자체 검토 메모

- 설계 문서의 모든 절(정리 모델·후보 신호·보호 규칙·worktree·명령·확인 단계·임계값·출력·테스트·저장소 통합·PRD 갱신)이 Task 1~11 에 대응된다.
- `Selection`·`Classify`·`gitx` 파서는 순수 함수라 단위 테스트로 검증하고, raw 모드 TUI 와 git 실제 호출은 Task 11 의 1회성 E2E 로 검증한다.
- 타입 이름은 `gitx.BranchRef`, `classify.Input`/`Classified`/`Result`/`Signal`, `pick.Selection`/`Mode` 로 모든 Task 에서 일관되게 쓰인다.
