# jg 무인자 실행 시 main working tree 최상단 고정 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git 저장소 안에서 `jg`를 인자 없이 실행하면 그 저장소의 main working tree 경로를 fzf 피커 최상단에 라벨과 함께 고정해 한 번에 돌아갈 수 있게 한다.

**Architecture:** main 패키지(`cmd/jg`)에 "현재 cwd 가 속한 저장소의 main working tree 루트"를 구하는 순수/통합 함수를 추가하고, fzf 패키지(`internal/fzf`)의 피커 입력을 "표시 영역 + 경로 영역" 두 열(탭 구분)로 바꿔 main 줄에만 라벨을 붙인다. fzf 는 표시 영역만 보여주고 선택된 줄에서 경로 영역만 떼어내 반환한다. main 고정은 무인자 실행에서만 일어난다.

**Tech Stack:** Go 1.23, 표준 라이브러리(`os/exec`, `strings`, `path/filepath`), 외부 명령 `git`·`fzf`, 테스트는 Go `testing` + 실제 임시 git 저장소.

**기준 문서(단일 기준):** 이 계획의 완료 검토는 이 계획서가 아니라 설계 문서
`docs/superpowers/specs/2026-06-09-jg-pin-main-worktree-design.md`(저장소 루트 기준)
에 직접 대조한다.

---

## 파일 구조

이 작업에서 만들거나 고치는 파일과 각자의 책임:

- **Create `apps/jg/cmd/jg/pinmain.go`** — main 고정 판정과 main working tree 경로 결정을 담는다. `shouldPinMain`(순수 판정), `resolvePinnedMain`(git 으로 main 루트 조회) 두 함수만 둔다. 기존 `cmd/jg/jgw.go` 의 `detectRepoRoot`·`splitMain`·`canonicalPath` 를 같은 패키지에서 재사용한다.
- **Create `apps/jg/cmd/jg/pinmain_test.go`** — 위 두 함수의 테스트. `shouldPinMain` 은 표 기반 순수 테스트, `resolvePinnedMain` 은 실제 임시 git 저장소로 검증한다.
- **Modify `apps/jg/internal/fzf/fzf.go`** — 피커 입력을 두 열(`표시\t경로`)로 바꾸고, main 고정 항목을 만드는 `buildPickerLines`, 선택 줄에서 경로를 떼는 `parseSelectedPath` 를 추가한다. `Run` 시그니처에 `pinnedMain` 인자를 더하고, `previewCmd` 의 placeholder 를 `{}` 에서 `{2}` 로 바꾼다.
- **Modify `apps/jg/internal/fzf/fzf_test.go`** — `buildPickerLines`·`parseSelectedPath` 단위 테스트를 추가한다.
- **Modify `apps/jg/internal/fzf/preview_test.go`** — `previewCmd` 가 `{2}` 를 쓰도록 바뀌므로, 그 테스트의 placeholder 치환을 `{2}` 로 맞춘다.
- **Modify `apps/jg/cmd/jg/main.go`** — `runJump` 이 무인자일 때 `resolvePinnedMain` 으로 고정 경로를 구해 `fzf.Run` 에 넘기도록 바꾸고, 추적 항목이 0개여도 고정 경로가 있으면 피커를 띄우도록 early-exit 조건을 보정한다. `fzf.Run` 호출부의 인자도 갱신한다.
- **Modify `apps/jg/README.md`, `apps/jg/docs/README_ko.md`** — 무인자 `jg` 의 main 고정 동작을 사용법·기능에 한 줄씩 추가한다.

각 작업은 빌드가 깨지지 않는 단위로 나뉘어 있다. Go 는 사용되지 않는 패키지 레벨 함수를 컴파일 오류로 보지 않으므로, 함수를 먼저 정의(Task 1~4)하고 나중에 배선(Task 6)해도 중간 빌드가 초록이다.

---

## Task 1: main 고정 판정 순수 함수 `shouldPinMain`

cwd 와 main 루트 경로가 주어졌을 때 고정할지 말지를 판정하는 순수 함수다. main 경로가 비었거나 cwd 가 이미 main 루트와 같은 자리이면 고정하지 않는다. 심볼릭 링크 차이로 인한 거짓 불일치를 막기 위해 기존 `canonicalPath`(jgw.go 에 있음) 로 양쪽을 정규화해 비교한다.

**Files:**
- Create: `apps/jg/cmd/jg/pinmain.go`
- Test: `apps/jg/cmd/jg/pinmain_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/cmd/jg/pinmain_test.go` 를 새로 만들고 아래 내용을 넣는다.

```go
package main

import "testing"

func TestShouldPinMain(t *testing.T) {
	cases := []struct {
		name          string
		cwd, mainPath string
		want          bool
	}{
		{"subdir pins", "/repo/sub", "/repo", true},
		{"at main root no pin", "/repo", "/repo", false},
		{"empty main no pin", "/repo/sub", "", false},
	}
	for _, tc := range cases {
		if got := shouldPinMain(tc.cwd, tc.mainPath); got != tc.want {
			t.Errorf("%s: shouldPinMain(%q, %q) = %v, want %v",
				tc.name, tc.cwd, tc.mainPath, got, tc.want)
		}
	}
}
```

존재하지 않는 경로(`/repo/sub` 등)는 `canonicalPath` 안의 `filepath.EvalSymlinks` 가 실패해 입력을 그대로 돌려주므로, 실제 디렉토리 없이도 문자열 비교로 판정이 검증된다.

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./cmd/jg/ -run TestShouldPinMain -v`
Expected: 컴파일 단계에서 `undefined: shouldPinMain` 로 실패. (구현이 없으므로 빌드 실패가 정상이며, 다음 단계에서 빈 구현이 아닌 최소 구현을 넣고 실제 통과를 확인한다.)

- [ ] **Step 3: 최소 구현 작성**

`apps/jg/cmd/jg/pinmain.go` 를 새로 만들고 아래 내용을 넣는다. 이 단계에서는 `import` 없이 `shouldPinMain` 만 둔다(`canonicalPath` 는 같은 패키지 jgw.go 에 있어 import 가 필요 없다).

```go
package main

// shouldPinMain 은 main working tree 를 피커 최상단에 고정할지 판정한다.
// main 경로가 비었거나 cwd 가 이미 main 루트와 같은 자리이면 고정하지 않는다.
// 수정 시 검토 관점: cwd 와 mainPath 는 표기(심볼릭 링크)가 어긋날 수 있으므로
// 반드시 canonicalPath 로 양쪽을 정규화한 뒤 비교한다. 한쪽만 정규화하면
// 같은 디렉토리를 다른 것으로 오판해 자기 자신을 고정 항목으로 띄운다.
func shouldPinMain(cwd, mainPath string) bool {
	if mainPath == "" {
		return false
	}
	return canonicalPath(cwd) != canonicalPath(mainPath)
}
```

- [ ] **Step 4: 테스트가 통과하는지 실행으로 확인**

Run: `cd apps/jg && go test ./cmd/jg/ -run TestShouldPinMain -v`
Expected: PASS (`TestShouldPinMain` 3개 케이스 통과)

- [ ] **Step 5: 커밋**

```bash
cd apps/jg
git add cmd/jg/pinmain.go cmd/jg/pinmain_test.go
git commit -m "feat(jg): main 고정 판정 함수 shouldPinMain 추가"
```

---

## Task 2: main working tree 경로 조회 `resolvePinnedMain`

cwd 가 속한 저장소의 main working tree 루트를 git 으로 조회한다. 저장소 밖이거나 worktree 목록을 못 얻거나 cwd 가 곧 main 루트이면 빈 문자열을 돌려준다. 저장소 판별은 `detectRepoRoot`, worktree 목록은 `worktree.List`, main 식별은 `splitMain`(모두 기존 코드) 을 재사용한다.

**Files:**
- Modify: `apps/jg/cmd/jg/pinmain.go`
- Test: `apps/jg/cmd/jg/pinmain_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/cmd/jg/pinmain_test.go` 의 import 블록과 테스트를 아래처럼 보강한다. 파일 맨 위 import 를 다음으로 교체한다.

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)
```

그리고 파일 끝에 git 저장소 헬퍼와 테스트를 추가한다.

```go
// newGitRepo 는 커밋 한 개를 가진 임시 git 저장소를 만들고 경로를 돌려준다.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestResolvePinnedMain(t *testing.T) {
	repo := newGitRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// cwd 가 저장소 하위 디렉토리이면 저장소 루트를 고정 대상으로 돌려준다.
	if got := resolvePinnedMain(sub); canonicalPath(got) != canonicalPath(repo) {
		t.Errorf("resolvePinnedMain(sub) = %q, want repo root %q", got, repo)
	}

	// cwd 가 곧 main 루트이면 빈 문자열.
	if got := resolvePinnedMain(repo); got != "" {
		t.Errorf("resolvePinnedMain(root) = %q, want empty", got)
	}

	// git 저장소 밖이면 빈 문자열.
	nonRepo := t.TempDir()
	if got := resolvePinnedMain(nonRepo); got != "" {
		t.Errorf("resolvePinnedMain(nonRepo) = %q, want empty", got)
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./cmd/jg/ -run TestResolvePinnedMain -v`
Expected: 컴파일 단계에서 `undefined: resolvePinnedMain` 로 실패.

- [ ] **Step 3: 최소 구현 작성**

`apps/jg/cmd/jg/pinmain.go` 에 `worktree` import 와 `resolvePinnedMain` 을 추가한다. 파일 전체를 아래로 교체한다.

```go
package main

import "github.com/silee-tools/jg/internal/worktree"

// shouldPinMain 은 main working tree 를 피커 최상단에 고정할지 판정한다.
// main 경로가 비었거나 cwd 가 이미 main 루트와 같은 자리이면 고정하지 않는다.
// 수정 시 검토 관점: cwd 와 mainPath 는 표기(심볼릭 링크)가 어긋날 수 있으므로
// 반드시 canonicalPath 로 양쪽을 정규화한 뒤 비교한다. 한쪽만 정규화하면
// 같은 디렉토리를 다른 것으로 오판해 자기 자신을 고정 항목으로 띄운다.
func shouldPinMain(cwd, mainPath string) bool {
	if mainPath == "" {
		return false
	}
	return canonicalPath(cwd) != canonicalPath(mainPath)
}

// resolvePinnedMain 은 cwd 가 속한 git 저장소의 main working tree 루트를
// 돌려준다. 저장소 밖이거나 main 을 못 구하거나 cwd 가 곧 main 루트이면
// 빈 문자열을 돌려준다. linked worktree 안에서도 git worktree 목록은 전체를
// 돌려주므로 splitMain 이 항상 main 루트를 집는다.
func resolvePinnedMain(cwd string) string {
	repoRoot, inRepo := detectRepoRoot(cwd)
	if !inRepo {
		return ""
	}
	wts, err := worktree.List(repoRoot)
	if err != nil || len(wts) == 0 {
		return ""
	}
	mainPath, _ := splitMain(wts)
	if !shouldPinMain(cwd, mainPath) {
		return ""
	}
	return mainPath
}
```

- [ ] **Step 4: 테스트가 통과하는지 실행으로 확인**

Run: `cd apps/jg && go test ./cmd/jg/ -run 'TestResolvePinnedMain|TestShouldPinMain' -v`
Expected: PASS (두 테스트 모두 통과)

- [ ] **Step 5: 커밋**

```bash
cd apps/jg
git add cmd/jg/pinmain.go cmd/jg/pinmain_test.go
git commit -m "feat(jg): cwd 의 main working tree 루트를 구하는 resolvePinnedMain 추가"
```

---

## Task 3: 피커 입력 빌더 `buildPickerLines`

피커에 넣을 줄 목록을 만든다. 각 줄은 "표시 영역"과 "경로 영역"으로 나뉜다. `pinnedMain` 이 비어 있지 않으면 그 경로를 `↑ main  ` 라벨과 함께 맨 앞에 고정하고, 본문에서 같은 경로를 제거해 중복을 막는다.

**Files:**
- Modify: `apps/jg/internal/fzf/fzf.go`
- Test: `apps/jg/internal/fzf/fzf_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/internal/fzf/fzf_test.go` 의 import 를 `entry` 패키지를 포함하도록 바꾼다. 파일 맨 위를 아래로 교체한다.

```go
package fzf

import (
	"testing"

	"github.com/silee-tools/jg/internal/entry"
)
```

그리고 파일 끝에 아래 테스트들을 추가한다.

```go
func TestBuildPickerLinesPinsMainWithLabel(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{
		{Path: "/home/tester/repos/a"},
		{Path: "/home/tester/repos/b"},
	}
	lines := buildPickerLines(entries, "/home/tester/repos/main", home)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	if lines[0].display != "↑ main  ~/repos/main" {
		t.Errorf("pinned display = %q", lines[0].display)
	}
	if lines[0].pathField != "~/repos/main" {
		t.Errorf("pinned pathField = %q", lines[0].pathField)
	}
}

func TestBuildPickerLinesDedupsPinnedFromBody(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{
		{Path: "/home/tester/repos/main"},
		{Path: "/home/tester/repos/a"},
	}
	lines := buildPickerLines(entries, "/home/tester/repos/main", home)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (pin + a, main deduped), got %d", len(lines))
	}
	for i, ln := range lines[1:] {
		if ln.pathField == "~/repos/main" {
			t.Errorf("main appeared again in body at index %d", i)
		}
	}
}

func TestBuildPickerLinesNoPin(t *testing.T) {
	home := "/home/tester"
	entries := []entry.Entry{{Path: "/home/tester/repos/a"}}
	lines := buildPickerLines(entries, "", home)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	if lines[0].display != "~/repos/a" || lines[0].pathField != "~/repos/a" {
		t.Errorf("no-pin line = %+v", lines[0])
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestBuildPickerLines -v`
Expected: 컴파일 단계에서 `undefined: buildPickerLines` (및 `pickerLine`) 로 실패.

- [ ] **Step 3: 최소 구현 작성**

`apps/jg/internal/fzf/fzf.go` 의 `shortenPath` 함수 정의 바로 위(또는 파일 끝 적당한 위치)에 아래를 추가한다.

```go
// pickerLine 은 fzf 한 줄의 표시 영역과 경로 영역을 나눠 담는다. 호출부가
// 둘을 탭으로 이어 fzf 에 넘기면, fzf 는 표시 영역만 보여주고 선택 줄에서
// 경로 영역만 떼어낸다.
type pickerLine struct {
	display   string
	pathField string
}

// buildPickerLines 는 fzf 에 넘길 줄 목록을 만든다. pinnedMain 이 비어 있지
// 않으면 그 경로를 라벨과 함께 맨 앞에 고정하고, 본문 목록에서 같은 경로를
// 제거해 중복을 막는다.
// 수정 시 검토 관점: 라벨 문구("↑ main  ")를 바꾸면 표시만 바뀌고 반환 경로는
// pathField 에서 떼므로 parseSelectedPath 와 짝이 깨지지 않는다. 단 display 와
// pathField 사이 구분에 쓰는 탭은 호출부 입력 포맷·parseSelectedPath 와 한 쌍이다.
func buildPickerLines(entries []entry.Entry, pinnedMain, home string) []pickerLine {
	var lines []pickerLine
	if pinnedMain != "" {
		short := shortenPath(pinnedMain, home)
		lines = append(lines, pickerLine{
			display:   "↑ main  " + short,
			pathField: short,
		})
	}
	for _, e := range entries {
		if pinnedMain != "" && e.Path == pinnedMain {
			continue
		}
		short := shortenPath(e.Path, home)
		lines = append(lines, pickerLine{display: short, pathField: short})
	}
	return lines
}
```

- [ ] **Step 4: 테스트가 통과하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestBuildPickerLines -v`
Expected: PASS (3개 테스트 통과)

- [ ] **Step 5: 커밋**

```bash
cd apps/jg
git add internal/fzf/fzf.go internal/fzf/fzf_test.go
git commit -m "feat(jg): main 고정 줄을 만드는 buildPickerLines 추가"
```

---

## Task 4: 선택 줄에서 경로 추출 `parseSelectedPath`

fzf 가 돌려준 선택 줄(`표시\t경로` 형식)에서 경로 영역만 떼어 절대 경로로 되돌린다. 탭이 없으면 줄 전체를 경로로 본다(고정이 없을 때도 같은 함수로 처리하기 위함).

**Files:**
- Modify: `apps/jg/internal/fzf/fzf.go`
- Test: `apps/jg/internal/fzf/fzf_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/internal/fzf/fzf_test.go` 파일 끝에 추가한다.

```go
func TestParseSelectedPath(t *testing.T) {
	home := "/home/tester"
	tests := []struct{ in, want string }{
		{"↑ main  ~/repos/main\t~/repos/main", "/home/tester/repos/main"},
		{"~/repos/a\t~/repos/a", "/home/tester/repos/a"},
		{"/opt/x\t/opt/x", "/opt/x"},
		{"~/repos/a", "/home/tester/repos/a"},
		{"", ""},
		{"  ~/repos/a\t~/repos/a  ", "/home/tester/repos/a"},
	}
	for _, tt := range tests {
		if got := parseSelectedPath(tt.in, home); got != tt.want {
			t.Errorf("parseSelectedPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestParseSelectedPath -v`
Expected: 컴파일 단계에서 `undefined: parseSelectedPath` 로 실패.

- [ ] **Step 3: 최소 구현 작성**

`apps/jg/internal/fzf/fzf.go` 의 `expandPath` 함수 정의 바로 아래에 추가한다.

```go
// parseSelectedPath 는 fzf 가 돌려준 선택 줄에서 경로 영역만 떼어 절대 경로로
// 되돌린다. 줄은 "표시\t경로" 형식이므로 마지막 탭 뒤를 경로로 본다. 탭이
// 없으면 줄 전체를 경로로 본다.
func parseSelectedPath(selected, home string) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return ""
	}
	if i := strings.LastIndex(selected, "\t"); i >= 0 {
		selected = selected[i+1:]
	}
	return expandPath(selected, home)
}
```

- [ ] **Step 4: 테스트가 통과하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestParseSelectedPath -v`
Expected: PASS (6개 케이스 통과)

- [ ] **Step 5: 커밋**

```bash
cd apps/jg
git add internal/fzf/fzf.go internal/fzf/fzf_test.go
git commit -m "feat(jg): 선택 줄에서 경로를 떼는 parseSelectedPath 추가"
```

---

## Task 5: `Run` 을 두 열 입력·고정 인자로 전환하고 preview placeholder 갱신

`fzf.Run` 이 `pinnedMain` 을 받아 `buildPickerLines` 로 입력을 만들고, 입력을 `표시\t경로` 형식으로 넘기며, 선택 결과를 `parseSelectedPath` 로 해석하도록 바꾼다. 두 열 입력에 맞춰 fzf 에 `--delimiter`·`--with-nth=1` 을 더하고, preview 가 경로 영역(`{2}`)을 참조하도록 `previewCmd` 와 그 테스트의 placeholder 를 함께 바꾼다.

이 단계에서 `Run` 시그니처가 바뀌므로 유일한 호출부인 `cmd/jg/main.go:311` 도 함께 고친다. 실제 고정 경로 계산은 Task 6 에서 배선하므로, 이 단계에서는 호출부에 빈 문자열을 넘겨 빌드를 초록으로 유지한다.

**Files:**
- Modify: `apps/jg/internal/fzf/fzf.go` (`Run`, `previewCmd`)
- Modify: `apps/jg/internal/fzf/preview_test.go` (placeholder 치환)
- Modify: `apps/jg/cmd/jg/main.go:311` (호출부 인자)

- [ ] **Step 1: preview 테스트의 placeholder 치환을 `{2}` 로 바꾼다**

`apps/jg/internal/fzf/preview_test.go` 의 `TestPreviewCmdResolvesFocusedPath` 안에서 아래 한 줄을

```go
	cmd := fzfSubstitute(previewCmd("/home/unused"), repo)
```

다음으로 교체한다(이 테스트의 입력은 한 줄이고 경로 영역이 `{2}` 이므로, repo 경로를 `{2}` 자리에 따옴표로 감싸 넣는다).

```go
	cmd := strings.ReplaceAll(previewCmd("/home/unused"), "{2}", "'"+repo+"'")
```

`worktreePreviewCmd` 를 쓰는 `TestWorktreePreviewCmdResolvesFocusedPath` 와 헬퍼 `fzfSubstitute` 는 건드리지 않는다(jgw picker 는 여전히 `{}` 를 쓴다).

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestPreviewCmdResolvesFocusedPath -v`
Expected: FAIL. `previewCmd` 가 아직 `{}` 를 쓰므로 `{2}` 치환이 일어나지 않아, preview 명령의 `p=` 가 빈 값/`{}` 리터럴이 되어 git 출력에 `branch: probe-branch` 가 나오지 않는다.

- [ ] **Step 3: `previewCmd` 가 `{2}` 를 쓰도록 바꾼다**

`apps/jg/internal/fzf/fzf.go` 의 `previewCmd` 안에서 `p={}` 를 `p={2}` 로 바꾼다. 함수 전체를 아래로 교체한다.

```go
// previewCmd builds the fzf preview command, expanding ~ to $HOME for git commands.
// 입력은 "표시\t경로" 두 열이므로 preview 는 경로 영역인 {2} 를 참조한다.
// 수정 시 검토 관점: 입력 열 구성(buildPickerLines·Run 의 입력 포맷)을 바꾸면
// 이 {2} 인덱스도 함께 맞춰야 한다.
func previewCmd(home string) string {
	// fzf 가 {2} 를 작은따옴표로 감싸 치환하므로 여기서 다시 따옴표로 감싸지
	// 않는다. leading ~ 만 home 경로로 치환하되 dash 같은 POSIX sh 에서도
	// 동작하도록 case 와 ${p#~} 만 쓴다. ${p/.../...} 는 bash·zsh 전용이라
	// dash 에서 "Bad substitution" 으로 깨진다.
	resolve := fmt.Sprintf(`p={2}; case "$p" in "~"*) p="%s${p#\~}";; esac`, home)
	return resolve + `; git -C "$p" log --oneline -5 2>/dev/null; echo; echo "branch: $(git -C "$p" branch --show-current 2>/dev/null)"; echo; git -C "$p" status --short 2>/dev/null | head -10`
}
```

- [ ] **Step 4: preview 테스트가 통과하는지 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestPreviewCmdResolvesFocusedPath -v`
Expected: PASS

- [ ] **Step 5: `Run` 시그니처와 본문을 바꾼다**

`apps/jg/internal/fzf/fzf.go` 의 `Run` 함수 전체를 아래로 교체한다.

```go
// Run launches fzf with the given entries and optional query. pinnedMain 이
// 비어 있지 않으면 그 경로를 라벨과 함께 피커 최상단에 고정한다.
// Returns the selected path or empty string if cancelled.
func Run(entries []entry.Entry, query, pinnedMain string) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("fzf not found. Install it: brew install fzf")
	}

	home, _ := os.UserHomeDir()
	lines := buildPickerLines(entries, pinnedMain, home)

	args := []string{
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--select-1",
		"--keep-right",
		"--wrap",
		"--delimiter=\t",
		"--with-nth=1",
		"--header=Git Repos",
		"--preview", previewCmd(home),
	}
	if query != "" {
		args = append(args, "--query", shortenPath(query, home))
	}

	cmd := exec.Command(fzfPath, args...)
	cmd.Stderr = os.Stderr

	var input strings.Builder
	for _, ln := range lines {
		fmt.Fprintf(&input, "%s\t%s\n", ln.display, ln.pathField)
	}
	cmd.Stdin = strings.NewReader(input.String())

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf exit 1 = no match, exit 130 = cancelled
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", err
	}

	return parseSelectedPath(string(out), home), nil
}
```

- [ ] **Step 6: 호출부 인자를 갱신한다(임시로 빈 고정)**

`apps/jg/cmd/jg/main.go` 의 `runJump` 안에서 아래 한 줄을

```go
	selected, err := fzf.Run(sorted, query)
```

다음으로 교체한다(Task 6 에서 빈 문자열을 실제 고정 경로로 바꾼다).

```go
	selected, err := fzf.Run(sorted, query, "")
```

- [ ] **Step 7: 패키지 전체 테스트와 빌드가 통과하는지 확인**

Run: `cd apps/jg && go test ./... && go build ./...`
Expected: 전체 PASS, 빌드 성공. (`Run` 시그니처 변경이 모든 호출부에 반영됨)

- [ ] **Step 8: 커밋**

```bash
cd apps/jg
git add internal/fzf/fzf.go internal/fzf/preview_test.go cmd/jg/main.go
git commit -m "feat(jg): fzf Run 을 두 열 입력·pinnedMain 인자로 전환"
```

---

## Task 6: `runJump` 배선과 early-exit 보정

무인자 실행일 때 `resolvePinnedMain` 으로 고정 경로를 구해 `fzf.Run` 에 넘긴다. 추적 항목이 0개여도 고정 경로가 있으면 피커를 띄우도록 "No entries" 조기 종료 조건을 `pinnedMain` 까지 고려하게 바꾼다.

**Files:**
- Modify: `apps/jg/cmd/jg/main.go` (`runJump`)

- [ ] **Step 1: `runJump` 본문을 교체한다**

`apps/jg/cmd/jg/main.go` 의 `runJump` 함수 전체를 아래로 교체한다.

```go
func runJump(queryArgs []string) {
	entries, err := entry.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	valid, _ := entry.FilterValid(entries, entry.ValidatePath)
	if len(valid) != len(entries) {
		_ = entry.Save(valid)
	}

	// 무인자 실행일 때만 현재 저장소의 main working tree 를 최상단에 고정한다.
	var pinnedMain string
	if len(queryArgs) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			pinnedMain = resolvePinnedMain(cwd)
		}
	}

	// 추적 항목이 없고 고정할 main 도 없으면 띄울 것이 없다.
	if len(valid) == 0 && pinnedMain == "" {
		fmt.Fprintln(os.Stderr, "No entries. cd into git repos to start tracking.")
		os.Exit(0)
	}

	query := strings.Join(queryArgs, " ")
	sorted := frecency.SortWithBoost(valid, query)

	selected, err := fzf.Run(sorted, query, pinnedMain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if selected == "" {
		os.Exit(1)
	}

	fmt.Println(selected)
}
```

- [ ] **Step 2: 전체 테스트·빌드·정적검사·포맷이 통과하는지 확인**

Run:
```bash
cd apps/jg
go test ./...
go vet ./...
go build ./...
mise run fmt-check
```
Expected: 전체 PASS, 빌드 성공, vet 무경고, gofmt 차이 없음.

- [ ] **Step 3: 커밋**

```bash
cd apps/jg
git add cmd/jg/main.go
git commit -m "feat(jg): 무인자 jg 에서 main working tree 를 피커 최상단에 고정"
```

---

## Task 7: E2E 수동 검증과 문서 갱신

단위 테스트는 mock 경계 밖 실제 fzf 렌더·반환을 보장하지 못하므로, 실제 git worktree 안에서 빌드한 바이너리를 1케이스 돌려 종료한다. 그리고 사용법·기능 문서를 갱신한다.

**Files:**
- Modify: `apps/jg/README.md`
- Modify: `apps/jg/docs/README_ko.md`

- [ ] **Step 1: 바이너리를 빌드한다**

Run: `cd apps/jg && mise run build`
Expected: `apps/jg/jg` 바이너리 생성.

- [ ] **Step 2: E2E(1) — linked worktree 에서 무인자 jg 가 main 루트를 반환하는지 검증**

추적 항목이 1개(고정된 main)뿐이면 fzf `--select-1` 이 UI 없이 즉시 그 항목을 선택하므로, PTY 없이 표준 출력으로 반환 경로를 확인할 수 있다. 아래를 그대로 실행한다.

```bash
cd apps/jg
export JG_BIN="$PWD/jg"   # E2E(2) expect 에서 $env(JG_BIN) 으로 참조
TMP="$(mktemp -d)"
git -C "$TMP" init -q
git -C "$TMP" commit -q --allow-empty -m init
git -C "$TMP" worktree add -q "$TMP/wt" -b feat
export XDG_STATE_HOME="$TMP/state"   # 빈 frecency store 로 격리
out="$(cd "$TMP/wt" && "$JG_BIN")"
# 반환 경로가 main working tree 루트($TMP)와 (심볼릭 링크 정규화 후) 일치해야 함
if [ "$(cd "$out" && pwd -P)" = "$(cd "$TMP" && pwd -P)" ]; then
  echo "E2E(1) PASS: jg returned main root"
else
  echo "E2E(1) FAIL: got '$out', want '$TMP'"
fi
```
Expected: `E2E(1) PASS: jg returned main root`

- [ ] **Step 3: E2E(2) — 라벨이 실제로 렌더되는지 PTY 로 검증**

항목이 둘 이상이면 fzf UI 가 뜨므로, `expect` 로 PTY 를 할당해 `↑ main` 라벨이 화면에 그려지는지 확인한다. 위 Step 2 의 `TMP`·`XDG_STATE_HOME`·`JG_BIN` 환경을 이어서 쓴다(같은 셸 세션). 다른 저장소 하나를 store 에 추가해 항목을 둘로 만든다.

```bash
OTHER="$(mktemp -d)"
git -C "$OTHER" init -q
git -C "$OTHER" commit -q --allow-empty -m init
"$JG_BIN" --add "$TMP"     # main 저장소 추적
"$JG_BIN" --add "$OTHER"   # 다른 저장소 추적
WT="$TMP/wt" expect -c '
  set timeout 10
  spawn env XDG_STATE_HOME=$env(XDG_STATE_HOME) sh -c "cd $env(WT) && $env(JG_BIN)"
  expect {
    "main" { puts "\nE2E(2) PASS: main label rendered" }
    timeout { puts "\nE2E(2) FAIL: label not seen"; exit 1 }
  }
  send "\r"
  expect eof
'
```
Expected: `E2E(2) PASS: main label rendered` 출력. (`expect` 패턴은 ANSI 이스케이프 때문에 화살표 글리프 대신 `main` 토큰으로 매칭한다.)

정리:
```bash
rm -rf "$TMP" "$OTHER"
unset XDG_STATE_HOME
```

- [ ] **Step 4: README 사용법·기능에 main 고정 동작을 추가한다**

`apps/jg/README.md` 의 Usage 코드블록에서

```
jg              # Interactive jump with fzf
```

를 다음으로 교체한다.

```
jg              # Interactive jump with fzf (inside a repo, its main worktree is pinned on top)
```

그리고 같은 파일 Features 목록의 맨 끝에 아래 한 줄을 추가한다.

```
- **Main worktree pinning**: Inside a git repo, `jg` with no arguments pins that repo's main working tree at the top of the picker for a quick return from a linked worktree or a subdirectory
```

- [ ] **Step 5: 한국어 문서에 같은 내용을 추가한다**

`apps/jg/docs/README_ko.md` 를 열어 기능 목록(또는 사용법 설명) 에 아래 한 줄을 추가한다(영어 README 의 기능 항목과 같은 자리의 한국어 대응).

```
- **main worktree 고정**: git 저장소 안에서 인자 없이 `jg` 를 실행하면 그 저장소의 main working tree 가 피커 최상단에 고정되어, linked worktree 나 하위 디렉토리에서 빠르게 돌아갈 수 있다
```

- [ ] **Step 6: 문서 변경 후 전체 게이트 재확인**

Run:
```bash
cd apps/jg
go test ./...
mise run build
mise run fmt-check
```
Expected: 전체 PASS, 빌드 성공, 포맷 차이 없음.

- [ ] **Step 7: 커밋**

```bash
cd apps/jg
git add README.md docs/README_ko.md
git commit -m "docs(jg): 무인자 jg 의 main worktree 고정 동작 문서화"
```

---

## 완료 후 검토 (설계 문서 대조)

모든 Task 를 마치면, 구현을 이 계획서가 아니라 설계 문서
`docs/superpowers/specs/2026-06-09-jg-pin-main-worktree-design.md`
의 "수용 기준" 항목에 한 줄씩 직접 대조한다.

- linked worktree 안 무인자 `jg` → 첫 항목이 main 경로 + 라벨: Task 3·Task 7 E2E(2)
- main 하위 디렉토리 무인자 `jg` → 첫 항목이 저장소 루트: Task 2 `TestResolvePinnedMain`(subdir)
- cwd 가 main 루트와 일치 → 고정 없음: Task 1·Task 2 `TestResolvePinnedMain`(root)
- 저장소 밖 → 고정 없음: Task 2 `TestResolvePinnedMain`(nonRepo)
- main 이 frecency 목록에도 있을 때 중복 없음: Task 3 `TestBuildPickerLinesDedupsPinnedFromBody`
- 고정 main 선택 시 라벨이 아닌 순수 경로 반환: Task 4 `TestParseSelectedPath` + Task 7 E2E(1)
- 쿼리 인자가 있으면 고정 없음: Task 6 `runJump`(고정은 `len(queryArgs)==0` 일 때만)

빠진 기준이 있으면 해당 Task 로 돌아가 보강한다.
```
