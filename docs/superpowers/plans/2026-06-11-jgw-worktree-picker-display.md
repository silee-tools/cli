# jgw worktree picker 이름 중심 표시 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** jgw 의 worktree 선택 단계에서 후보를 폴더 경로 대신 worktree 이름(필요할 때만 브랜치)으로 보여주고, fzf fuzzy 검색은 그 표시 텍스트에 걸리되 선택 결과로는 worktree 실제 경로를 출력하도록 바꾼다.

**Architecture:** worktree 선택 단계만 전용 fzf 진입점으로 분리한다. 새 진입점은 입력 각 줄을 `<인덱스>\t<표시텍스트>` 두 필드로 만들어 fzf 에 넘기고, 인덱스 필드는 화면·검색에서 숨긴다. 사용자가 고른 줄의 인덱스로 worktree 슬라이스를 역조회해 경로를 반환하므로, 표시 텍스트가 겹쳐도 반환이 어긋나지 않는다. worktree 단계에는 preview 패널을 띄우지 않는다. 저장소(repo) 선택 단계는 기존 진입점(`RunWorktreePicker`)을 그대로 쓰며 경로 표시와 preview 를 유지한다.

**Tech Stack:** Go (표준 라이브러리 `os/exec`, `strconv`, `path`, `path/filepath`, `strings`), fzf, git worktree porcelain. 테스트는 Go `testing` + 라벨/입력/파싱 순수 함수 단위 테스트, 마지막에 `expect` 로 PTY 를 할당한 1회성 E2E.

---

## 배경 참조

- 설계 기준 문서(단일 기준): `docs/superpowers/specs/2026-06-11-jgw-worktree-picker-display-design.md`. 완료 검토는 이 plan 이 아니라 이 spec 에 직접 대조한다.
- 변경 대상 파일은 모두 `apps/jg/` 아래에 있다. 작업·테스트 명령은 그 디렉토리에서 실행한다.

## 현재 구조 요약 (작업 전 사실)

- `internal/worktree/worktree.go`: `type Worktree struct { Path string; Branch string; IsMain bool }`. `List(repoPath)` 가 `git worktree list --porcelain` 을 파싱해 `[]Worktree` 를 돌려준다. detached worktree 는 `Branch` 가 빈 문자열이다.
- `internal/fzf/jgwfzf.go`: `RunWorktreePicker(WorktreePickerInput)` 가 `Candidates []string`(단축 경로 목록), `CurrentPath string`, `StepHeader`, `OriginLine` 을 받아 fzf 를 띄우고 선택된 경로를 그대로 돌려준다. preview 는 `worktreePreviewCmd(home)` 가 만든다.
- `cmd/jg/jgw.go`: `runJgwFlowA` 와 `runJgwFlowB` 가 `RunWorktreePicker` 를 호출한다. flow A 는 worktree 단계 한 번, flow B 는 1단계에서 **repo 선택**으로 `RunWorktreePicker` 를 호출하고(경로 목록), 2단계에서 worktree 선택으로 다시 호출한다. `splitMain(wts)` 는 `(path, branch)` 를, `splitCurrent(wts, cwd)` 는 `(candidates []string, current string)` 을 돌려준다.
- 핵심 제약: `RunWorktreePicker` 는 worktree 단계와 flow B 의 **repo 단계 둘 다** 쓴다. 따라서 이 함수를 worktree 전용으로 바꾸면 repo 단계가 깨진다. worktree 단계만 새 진입점으로 분리하고 기존 함수는 그대로 둔다.

## 파일 구조 (생성/수정 대상)

- 수정: `apps/jg/internal/fzf/jgwfzf.go` — worktree 전용 새 진입점 `RunWorktreeListPicker`, 입력 타입 `WorktreeListPickerInput`, 순수 함수 `worktreeLabel`·`buildWorktreeInput`·`selectedWorktreeIndex` 추가. 기존 `RunWorktreePicker`·`worktreePreviewCmd` 는 건드리지 않는다.
- 수정: `apps/jg/internal/fzf/jgwfzf_test.go` — `worktreeLabel`·`buildWorktreeInput`·`selectedWorktreeIndex` 단위 테스트 추가. 기존 `TestWorktreePreviewCmdIncludesBranchAndLog` 는 그대로 둔다(preview 명령이 repo 단계에서 계속 쓰이므로).
- 수정: `apps/jg/cmd/jg/jgw.go` — `splitCurrent` 반환 타입을 `([]worktree.Worktree, *worktree.Worktree)` 로 바꾸고, flow A·flow B 의 worktree 단계가 `RunWorktreeListPicker` 를 쓰도록 전환. flow B 의 repo 단계는 기존 `RunWorktreePicker` 호출을 유지.
- 수정: `apps/jg/cmd/jg/main_test.go` — `splitCurrent` 반환 타입 변경에 맞춰 `TestSplitCurrentSeparatesCwd`·`TestSplitCurrentDoesNotFalseMatchPrefix`·`TestSplitCurrentMatchesThroughSymlink` 갱신.

`internal/worktree` 패키지, `internal/fzf/fzf.go`(jg 메인 repo picker), `completions/`, `plugin/` 은 이번 변경에서 손대지 않는다(표시 텍스트만 바뀌고 명령·플래그·자동완성 후보는 그대로다).

---

## Task 1: worktree 표시 라벨 순수 함수

worktree 한 개를 받아 picker 에 보여줄 한 줄 라벨을 만드는 순수 함수다. 이름은 경로 basename, main 은 `▸ ` 마커, 브랜치 basename 이 이름과 같으면 브랜치 생략, 다르면 덧붙이고, detached(브랜치 빈 문자열)는 `(detached)` 를 덧붙인다.

**Files:**
- Modify: `apps/jg/internal/fzf/jgwfzf.go`
- Test: `apps/jg/internal/fzf/jgwfzf_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/internal/fzf/jgwfzf_test.go` 에 import 와 테스트를 추가한다. 파일 상단 import 블록에 `"github.com/silee-tools/jg/internal/worktree"` 가 없으면 추가한다.

```go
func TestWorktreeLabel(t *testing.T) {
	cases := []struct {
		name string
		wt   worktree.Worktree
		want string
	}{
		{
			name: "main 은 마커와 브랜치를 함께 보여준다",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true},
			want: "▸ acme-app  main",
		},
		{
			name: "브랜치 basename 이 이름과 같으면 이름만",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/ABC-101-login-timeout", Branch: "feature/ABC-101-login-timeout", IsMain: false},
			want: "  ABC-101-login-timeout",
		},
		{
			name: "브랜치 basename 이 이름과 다르면 브랜치 덧붙임",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/wt-foo", Branch: "feature/bar", IsMain: false},
			want: "  wt-foo  feature/bar",
		},
		{
			name: "detached 는 (detached) 덧붙임",
			wt:   worktree.Worktree{Path: "/home/me/repos/acme-app/.worktrees/hotfix", Branch: "", IsMain: false},
			want: "  hotfix  (detached)",
		},
		{
			name: "main 이면서 이름과 브랜치 basename 이 같으면 마커만",
			wt:   worktree.Worktree{Path: "/home/me/repos/main", Branch: "main", IsMain: true},
			want: "▸ main",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := worktreeLabel(c.wt); got != c.want {
				t.Errorf("worktreeLabel = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestWorktreeLabel -v`
Expected: 컴파일 실패 또는 `undefined: worktreeLabel`. 함수가 아직 없어 실패한다.

- [ ] **Step 3: 최소 구현 추가**

`apps/jg/internal/fzf/jgwfzf.go` 의 import 블록에 `"path"` 와 `"path/filepath"` 를 추가하고(이미 있으면 생략), 아래 함수를 추가한다.

```go
// worktreeLabel 은 worktree 한 개를 picker 에 보여줄 한 줄 라벨로 만든다.
// 이름은 경로 basename 이고, 원본(main) 은 "▸ " 마커를, 나머지는 같은 폭의
// 공백을 앞에 둔다. 브랜치 basename 이 이름과 같으면 중복이므로 브랜치를
// 생략하고, 다르면 이름 뒤에 공백 두 칸으로 브랜치를 잇는다. 브랜치가 없는
// detached worktree 는 "(detached)" 를 덧붙인다.
// 수정 시 검토 관점: 이 라벨은 buildWorktreeInput 이 "<인덱스>\t<라벨>" 로
// 엮어 fzf 에 넘기고 fzf 는 라벨만 보여주므로, 라벨 안에 탭 문자를 넣지 않는다.
func worktreeLabel(w worktree.Worktree) string {
	name := filepath.Base(w.Path)
	marker := "  "
	if w.IsMain {
		marker = "▸ "
	}
	if w.Branch == "" {
		return marker + name + "  (detached)"
	}
	if path.Base(w.Branch) == name {
		return marker + name
	}
	return marker + name + "  " + w.Branch
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run TestWorktreeLabel -v`
Expected: PASS (5개 서브테스트 모두).

- [ ] **Step 5: 커밋**

```bash
cd apps/jg && git add internal/fzf/jgwfzf.go internal/fzf/jgwfzf_test.go
git commit -m "feat(jg): worktree picker 라벨 순수 함수 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: 인덱스 동행 입력 빌더와 선택 파싱 순수 함수

fzf 에 넘길 입력 문자열을 만드는 `buildWorktreeInput` 과, fzf 가 돌려준 선택 줄에서 맨 앞 인덱스 필드를 떼는 `selectedWorktreeIndex` 를 추가한다. 둘 다 순수 함수라 단위 테스트로 동행 규약을 고정한다.

**Files:**
- Modify: `apps/jg/internal/fzf/jgwfzf.go`
- Test: `apps/jg/internal/fzf/jgwfzf_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/jg/internal/fzf/jgwfzf_test.go` 에 추가한다.

```go
func TestBuildWorktreeInput(t *testing.T) {
	cur := worktree.Worktree{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true}
	in := WorktreeListPickerInput{
		Current: &cur,
		Candidates: []worktree.Worktree{
			{Path: "/home/me/repos/acme-app/.worktrees/ABC-101-login-timeout", Branch: "feature/ABC-101-login-timeout"},
			{Path: "/home/me/repos/acme-app/.worktrees/ABC-102-upload-retry", Branch: "feature/ABC-102-upload-retry"},
		},
	}
	input, headerLines := buildWorktreeInput(in)
	if headerLines != 1 {
		t.Fatalf("headerLines = %d, want 1 (Current 가 있으면 헤더 줄 1개)", headerLines)
	}
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (헤더 1 + 후보 2)\n%s", len(lines), input)
	}
	// 헤더 줄: 선택 불가이므로 인덱스 자리는 -1, 라벨은 current 라벨
	if lines[0] != "-1\t▸ acme-app  main" {
		t.Errorf("header line = %q, want %q", lines[0], "-1\t▸ acme-app  main")
	}
	// 후보 줄: 인덱스가 0 부터 슬라이스 순서대로
	if lines[1] != "0\t  ABC-101-login-timeout" {
		t.Errorf("candidate[0] = %q", lines[1])
	}
	if lines[2] != "1\t  ABC-102-upload-retry" {
		t.Errorf("candidate[1] = %q", lines[2])
	}
}

func TestBuildWorktreeInputNoCurrent(t *testing.T) {
	in := WorktreeListPickerInput{
		Candidates: []worktree.Worktree{
			{Path: "/home/me/repos/acme-app", Branch: "main", IsMain: true},
		},
	}
	input, headerLines := buildWorktreeInput(in)
	if headerLines != 0 {
		t.Fatalf("headerLines = %d, want 0 (Current 없음)", headerLines)
	}
	if got := strings.TrimRight(input, "\n"); got != "0\t▸ acme-app  main" {
		t.Errorf("input = %q", got)
	}
}

func TestSelectedWorktreeIndex(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOk bool
	}{
		{"0\t  ABC-101-login-timeout\n", 0, true},
		{"2\t▸ acme-app  main", 2, true},
		{"", 0, false},
		{"notanumber\tlabel", 0, false},
	}
	for _, c := range cases {
		got, ok := selectedWorktreeIndex(c.in)
		if got != c.want || ok != c.wantOk {
			t.Errorf("selectedWorktreeIndex(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.wantOk)
		}
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run 'TestBuildWorktreeInput|TestSelectedWorktreeIndex' -v`
Expected: 컴파일 실패 — `WorktreeListPickerInput`, `buildWorktreeInput`, `selectedWorktreeIndex` 미정의.

- [ ] **Step 3: 최소 구현 추가**

`apps/jg/internal/fzf/jgwfzf.go` 의 import 블록에 `"strconv"` 를 추가하고, 아래 타입과 함수를 추가한다. 기존 `WorktreePickerInput` 은 그대로 둔다.

```go
// WorktreeListPickerInput 은 worktree 선택 단계 전용 picker 호출 파라미터다.
// 후보는 경로 문자열이 아니라 worktree 구조체로 받아, 라벨을 이름 중심으로
// 그리고 선택 결과는 인덱스로 역조회해 경로를 돌려준다.
type WorktreeListPickerInput struct {
	Candidates []worktree.Worktree  // 선택 가능한 worktree 후보 (현재 위치 제외)
	Current    *worktree.Worktree   // 현재 위치한 worktree. nil 이 아니면 헤더 줄로 고정 표시
	StepHeader string               // "[1/1 worktree 선택]" / "[2/2 worktree 선택]"
	OriginLine string               // "원본: <path> (<branch>)"
}

// buildWorktreeInput 은 fzf 에 넘길 입력 문자열과 헤더 줄 수를 만든다. 각 줄은
// "<인덱스>\t<라벨>" 이며, 인덱스는 Candidates 슬라이스의 자리값이다. Current 가
// 있으면 맨 앞에 인덱스 -1 의 헤더 줄을 두고 headerLines 를 1 로 돌려준다(이 줄은
// fzf 의 --header-lines 로 고정돼 선택되지 않으므로 인덱스 값은 쓰이지 않는다).
func buildWorktreeInput(in WorktreeListPickerInput) (input string, headerLines int) {
	var b strings.Builder
	if in.Current != nil {
		fmt.Fprintf(&b, "-1\t%s\n", worktreeLabel(*in.Current))
		headerLines = 1
	}
	for i, w := range in.Candidates {
		fmt.Fprintf(&b, "%d\t%s\n", i, worktreeLabel(w))
	}
	return b.String(), headerLines
}

// selectedWorktreeIndex 는 fzf 가 돌려준 선택 줄에서 맨 앞 인덱스 필드를 떼어
// 정수로 바꾼다. 줄은 "<인덱스>\t<라벨>" 형식이라 첫 탭 앞을 인덱스로 본다.
// 비어 있거나 인덱스가 정수가 아니면 ok=false 를 돌려준다.
func selectedWorktreeIndex(selected string) (int, bool) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return 0, false
	}
	field := selected
	if i := strings.Index(selected, "\t"); i >= 0 {
		field = selected[:i]
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0, false
	}
	return n, true
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/jg && go test ./internal/fzf/ -run 'TestBuildWorktreeInput|TestSelectedWorktreeIndex' -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd apps/jg && git add internal/fzf/jgwfzf.go internal/fzf/jgwfzf_test.go
git commit -m "feat(jg): worktree picker 인덱스 동행 입력/파싱 함수 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: worktree 전용 picker 진입점 RunWorktreeListPicker

앞의 순수 함수들을 묶어 fzf 를 실제로 띄우는 진입점을 추가한다. preview 옵션을 넘기지 않고, 인덱스 필드를 `--with-nth=2..` 로 숨기며, 선택 줄의 인덱스로 후보 경로를 돌려준다. fzf 실행 자체는 단위 테스트가 어려우므로(외부 프로세스·TTY), 동작 검증은 Task 5 의 E2E 가 맡는다. 여기서는 함수가 컴파일되고 기존 테스트가 깨지지 않는 것까지만 확인한다.

**Files:**
- Modify: `apps/jg/internal/fzf/jgwfzf.go`

- [ ] **Step 1: 진입점 함수 추가**

`apps/jg/internal/fzf/jgwfzf.go` 에 추가한다. import 에 `"os"` 와 `"os/exec"` 는 이미 있다(기존 `RunWorktreePicker` 가 쓴다).

```go
// RunWorktreeListPicker 는 worktree 선택 단계 전용 fzf picker 를 띄우고 선택된
// worktree 의 경로를 돌려준다. 취소 시 빈 문자열을 돌려준다. 입력은 인덱스를
// 숨김 필드로 동행시킨 "<인덱스>\t<라벨>" 형식이며, --with-nth=2.. 로 인덱스를
// 화면과 검색에서 빼고 라벨만 보여준다. worktree 단계에는 preview 를 띄우지 않는다.
// 수정 시 검토 관점: --delimiter·--with-nth 와 buildWorktreeInput 의 필드 구성,
// selectedWorktreeIndex 의 파싱은 한 묶음이다. 한쪽을 바꾸면 나머지도 맞춘다.
func RunWorktreeListPicker(in WorktreeListPickerInput) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("fzf not found. Install it: brew install fzf")
	}

	headerParts := []string{}
	if in.StepHeader != "" {
		headerParts = append(headerParts, in.StepHeader)
	}
	if in.OriginLine != "" {
		headerParts = append(headerParts, in.OriginLine)
	}
	header := strings.Join(headerParts, "\n")

	args := []string{
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--select-1",
		"--wrap",
		"--delimiter=\t",
		"--with-nth=2..",
	}
	if header != "" {
		args = append(args, "--header", header)
	}

	input, headerLines := buildWorktreeInput(in)
	if headerLines > 0 {
		args = append(args, "--header-lines=1")
	}

	cmd := exec.Command(fzfPath, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", err
	}

	idx, ok := selectedWorktreeIndex(string(out))
	if !ok || idx < 0 || idx >= len(in.Candidates) {
		return "", nil
	}
	return in.Candidates[idx].Path, nil
}
```

- [ ] **Step 2: 패키지 컴파일·기존 테스트 확인**

Run: `cd apps/jg && go build ./... && go test ./internal/fzf/ -v`
Expected: 빌드 성공, `internal/fzf` 의 모든 테스트(기존 preview 테스트 포함) PASS.

- [ ] **Step 3: 커밋**

```bash
cd apps/jg && git add internal/fzf/jgwfzf.go
git commit -m "feat(jg): worktree 전용 picker 진입점 추가 (preview 없음)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: jgw.go 호출부를 새 진입점으로 전환

`splitCurrent` 반환 타입을 worktree 구조체 기반으로 바꾸고, flow A 와 flow B 의 worktree 단계가 `RunWorktreeListPicker` 를 쓰게 한다. flow B 의 1단계 repo 선택은 기존 `RunWorktreePicker`(경로 목록·preview) 를 그대로 유지한다. `splitCurrent` 시그니처가 바뀌므로 `main_test.go` 의 해당 테스트도 같은 단계에서 갱신한다.

**Files:**
- Modify: `apps/jg/cmd/jg/jgw.go`
- Test: `apps/jg/cmd/jg/main_test.go`

- [ ] **Step 1: main_test.go 의 splitCurrent 테스트를 새 시그니처로 바꾼다 (실패 유도)**

`apps/jg/cmd/jg/main_test.go` 에서 세 테스트를 아래로 교체한다. 새 시그니처는 `splitCurrent(wts, cwd) ([]worktree.Worktree, *worktree.Worktree)` 다. 파일 import 에 `"github.com/silee-tools/jg/internal/worktree"` 가 없으면 추가한다(기존 `splitMain` 테스트가 `worktree.Worktree` 리터럴을 쓰므로 이미 있을 가능성이 높다).

```go
func TestSplitCurrentSeparatesCwd(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo", Branch: "main", IsMain: true},
		{Path: "/repo-wt1", Branch: "feature/x"},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current == nil || current.Path != "/repo-wt1" {
		t.Fatalf("current = %v, want /repo-wt1", current)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo" {
		t.Errorf("candidates = %v, want [/repo]", candidates)
	}
}

func TestSplitCurrentDoesNotFalseMatchPrefix(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo-wt1", Branch: "feature/x"},
		{Path: "/repo-wt10", Branch: "feature/y"},
	}
	candidates, current := splitCurrent(wts, "/repo-wt1/subdir")
	if current == nil || current.Path != "/repo-wt1" {
		t.Fatalf("current = %v, want /repo-wt1 (prefix /repo-wt10 과 혼동 금지)", current)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo-wt10" {
		t.Errorf("candidates = %v, want [/repo-wt10]", candidates)
	}
}

func TestSplitCurrentMatchesThroughSymlink(t *testing.T) {
	// 실제 디렉토리와 심볼릭 링크를 만들어, 셸이 넘긴 논리 경로(심볼릭)와 git 이
	// 돌려준 정규 경로가 어긋나도 현재 worktree 를 찾아내는지 본다.
	realDir := t.TempDir()
	wtReal := filepath.Join(realDir, "wt-real")
	if err := os.Mkdir(wtReal, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(realDir, "wt-link")
	if err := os.Symlink(wtReal, link); err != nil {
		t.Fatal(err)
	}
	wts := []worktree.Worktree{
		{Path: "/repo", Branch: "main", IsMain: true},
		{Path: wtReal, Branch: "feature/x"},
	}
	cwd := filepath.Join(link, "subdir")
	candidates, current := splitCurrent(wts, cwd)
	if current == nil || current.Path != wtReal {
		t.Fatalf("current = %v, want %s", current, wtReal)
	}
	if len(candidates) != 1 || candidates[0].Path != "/repo" {
		t.Errorf("candidates = %v, want [/repo]", candidates)
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 실행으로 확인**

Run: `cd apps/jg && go test ./cmd/jg/ -run TestSplitCurrent -v`
Expected: 컴파일 실패 — `splitCurrent` 가 아직 `([]string, string)` 을 돌려줘 타입이 맞지 않는다.

- [ ] **Step 3: splitCurrent 를 새 시그니처로 구현**

`apps/jg/cmd/jg/jgw.go` 의 `splitCurrent` 를 아래로 교체한다.

```go
func splitCurrent(wts []worktree.Worktree, cwd string) (candidates []worktree.Worktree, current *worktree.Worktree) {
	// os.Getwd() 는 셸이 넘긴 논리 경로를, git worktree list 는 심볼릭 링크를
	// 푼 정규 경로를 돌려준다. 두 표기가 어긋나면 현재 worktree 를 못 찾으므로
	// 양쪽을 정규화한 뒤 비교한다. 반환값은 원본 Worktree 를 유지한다.
	cwdCanon := canonicalPath(cwd)
	for i := range wts {
		wPathCanon := canonicalPath(wts[i].Path)
		if cwdCanon == wPathCanon || strings.HasPrefix(cwdCanon, wPathCanon+string(os.PathSeparator)) {
			current = &wts[i]
			continue
		}
		candidates = append(candidates, wts[i])
	}
	return
}
```

- [ ] **Step 4: flow A 의 worktree 단계 호출을 새 진입점으로 교체**

`apps/jg/cmd/jg/jgw.go` 의 `runJgwFlowA` 에서, `splitCurrent` 결과를 쓰는 부분과 picker 호출을 아래로 맞춘다. `len(candidates) == 0 && current != ""` 비교는 `current != nil` 로, picker 호출은 `RunWorktreeListPicker` 로 바뀐다.

```go
	mainPath, mainBranch := splitMain(wts)
	candidates, current := splitCurrent(wts, cwd)
	if len(candidates) == 0 && current != nil {
		// 사용자가 유일한 worktree 안에 이미 있음 — 점프 없음
		os.Exit(0)
	}
	if len(candidates) == 0 {
		if mainPath == "" {
			fmt.Fprintln(os.Stderr, "jgw: cannot resolve main working tree")
			os.Exit(1)
		}
		fmt.Println(mainPath)
		_ = entry.AddOrUpdate(mainPath)
		return
	}
	selected, err := fzf.RunWorktreeListPicker(fzf.WorktreeListPickerInput{
		Candidates: candidates,
		Current:    current,
		StepHeader: stepHeader(1, 1, "worktree 선택"),
		OriginLine: fmt.Sprintf("원본: %s (%s)", mainPath, mainBranch),
	})
	if err != nil || selected == "" {
		os.Exit(1)
	}
	fmt.Println(selected)
	_ = wtstore.AddOrUpdate(selected)
	_ = entry.AddOrUpdate(repoRoot)
```

- [ ] **Step 5: flow B 의 worktree 단계(2단계) 호출을 새 진입점으로 교체**

`apps/jg/cmd/jg/jgw.go` 의 `runJgwFlowB` 에서, 2단계 worktree 선택 부분을 아래로 맞춘다. 기존에 `candidates := make([]string, ...)` 로 경로 목록을 만들던 블록을 없애고 `wts` 를 그대로 넘긴다. **이 함수 위쪽 1단계의 repo 선택 `RunWorktreePicker` 호출(`Candidates: paths`)은 건드리지 않는다.**

```go
	selected, err := fzf.RunWorktreeListPicker(fzf.WorktreeListPickerInput{
		Candidates: wts,
		Current:    nil,
		StepHeader: stepHeader(2, 2, "worktree 선택"),
		OriginLine: fmt.Sprintf("원본: %s (%s)", mainPath, mainBranch),
	})
	if err != nil || selected == "" {
		os.Exit(1)
	}
	fmt.Println(selected)
	_ = wtstore.AddOrUpdate(selected)
	_ = entry.AddOrUpdate(repoPicked)
```

- [ ] **Step 6: 전체 단위 테스트와 빌드 게이트 실행**

Run: `cd apps/jg && go build ./... && go vet ./... && go test ./...`
Expected: 빌드·vet 통과, 모든 패키지 테스트 PASS. (vet 은 트랜스파일러가 아니라 컴파일러 기반 검사이므로 미사용 import·타입 불일치를 잡는 게이트로 함께 돌린다.)

- [ ] **Step 7: 커밋**

```bash
cd apps/jg && git add cmd/jg/jgw.go cmd/jg/main_test.go
git commit -m "feat(jg): jgw worktree 단계를 이름 중심 picker 로 전환

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: 1회성 E2E 검증 (expect PTY)

worktree picker 는 stdin 이 TTY 일 때만 fzf 가 정상 동작하므로, 비-TTY 에이전트 셸에서 파이프로 입력을 주면 경로를 타지 않는다. `expect` 로 PTY 를 할당해 실제 picker 를 띄우고, 행이 이름 중심으로 보이는지와 선택 결과로 worktree 경로가 출력되는지를 한 번 확인한다. 이 검증은 영구 회귀 테스트가 아니라 변경 경로 1케이스의 수동 스모크다.

**Files:**
- 없음(검증만 수행). 검증 스크립트는 세션 임시 디렉토리에 둔다.

- [ ] **Step 1: 검증용 임시 git repo 와 worktree 를 만든다**

```bash
WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}"
mkdir -p "$WORK"
cd apps/jg && go build -o "$WORK/jg" ./cmd/jg
ln -sf "$WORK/jg" "$WORK/jgw"

REPO="$WORK/acme-app"
rm -rf "$REPO"
git init -q "$REPO"
git -C "$REPO" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
git -C "$REPO" branch -M main
git -C "$REPO" worktree add -q -b feature/ABC-101-login-timeout "$REPO/.worktrees/ABC-101-login-timeout"
git -C "$REPO" worktree add -q -b feature/ABC-102-upload-retry "$REPO/.worktrees/ABC-102-upload-retry"
git -C "$REPO" worktree list
```

Expected: worktree 3개(main + 2개)가 출력된다.

- [ ] **Step 2: expect 로 picker 를 띄워 표시·선택을 검증한다**

저장소 안에서 `jgw`(인자 없음) 를 부르면 flow A(worktree 단계 한 번)가 돈다. 현재 위치를 main 으로 두면 두 후보가 picker 에 뜬다. `ABC-102` 를 타이핑해 좁히고 Enter 로 고른 뒤, 바이너리가 그 worktree 경로를 stdout 에 출력하는지 본다.

```bash
WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}"
REPO="$WORK/acme-app"
expect -c "
set timeout 10
cd \"$REPO\"
spawn \"$WORK/jgw\"
expect {
  -re {ABC-101-login-timeout} { }
  timeout { puts \"FAIL: 후보 라벨이 이름 중심으로 안 보임\"; exit 1 }
}
send \"ABC-102\"
send \"\r\"
expect {
  -re {/\\.worktrees/ABC-102-upload-retry} { puts \"\nOK: 선택 결과로 worktree 경로 출력\"; }
  timeout { puts \"FAIL: 선택 경로 미출력\"; exit 1 }
}
expect eof
"
```

Expected: picker 에 `ABC-101-login-timeout`·`ABC-102-upload-retry` 가 이름 중심 라벨로 뜨고(폴더 경로 `.worktrees/...` 가 후보 줄에 풀로 보이지 않음), `ABC-102` 로 좁혀 Enter 하면 stdout 마지막에 그 worktree 의 절대 경로(`.../.worktrees/ABC-102-upload-retry`)가 출력된다. `OK:` 두 줄이 보이면 통과다.

- [ ] **Step 3: 검증 디렉토리 정리**

```bash
WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}"
git -C "$WORK/acme-app" worktree prune 2>/dev/null
rm -rf "$WORK"
```

Expected: 세션 임시 디렉토리만 통째로 제거된다.

- [ ] **Step 4: E2E 결과를 PR 본문용으로 기록**

위 expect 실행 출력(picker 라벨 캡처와 `OK:` 두 줄)을 PR 본문의 검증 증거로 남긴다. tdd-principles 의 "변경 Service 경로 1케이스 E2E" 조항을 이 스모크로 충족한다.

---

## Self-Review (작성자 체크리스트 — spec 대조)

설계 기준 문서 `docs/superpowers/specs/2026-06-11-jgw-worktree-picker-display-design.md` 의 각 요구를 어느 Task 가 구현하는지 대조한다.

- 행을 이름 중심으로 표시 → Task 1(`worktreeLabel`).
- 브랜치 basename == 이름이면 브랜치 생략, 다르면 덧붙임, detached 는 `(detached)` → Task 1.
- 원본(main) 에 `▸` 마커 → Task 1.
- fuzzy 검색이 표시 텍스트에 걸리고 인덱스 필드는 검색에서 제외 → Task 3(`--with-nth=2..`).
- 선택 시 worktree 실제 경로 출력, 셸 cd·frecency 그대로 → Task 3(인덱스 역조회)·Task 4(`fmt.Println(selected)` + `wtstore`/`entry` 유지).
- 인덱스 동행 `<인덱스>\t<표시텍스트>` 두 필드 → Task 2(`buildWorktreeInput`)·Task 3.
- 저장소 picker 는 경로 표시 유지 → Task 4 가 flow B 1단계 `RunWorktreePicker` 호출을 안 건드림.
- worktree picker 에 preview 없음, preview 명령 정의는 repo 단계용으로 유지 → Task 3(새 진입점 preview 미사용), 기존 `worktreePreviewCmd`·그 테스트 보존.
- 현재 위치 worktree 도 같은 라벨로 헤더 고정 → Task 2(`buildWorktreeInput` 의 Current 헤더 줄)·Task 1.

**Placeholder 점검:** 모든 코드 step 에 실제 코드와 실제 명령·기대 출력이 들어 있다. TBD·"적절히 처리" 류 없음.

**타입 일관성:** `WorktreeListPickerInput`/`RunWorktreeListPicker`/`worktreeLabel`/`buildWorktreeInput`/`selectedWorktreeIndex` 이름이 Task 1~4 에서 동일하게 쓰인다. `splitCurrent` 새 반환 타입 `([]worktree.Worktree, *worktree.Worktree)` 이 Task 4 의 호출부(`current != nil`)와 main_test.go 갱신에 일관되게 반영됐다.