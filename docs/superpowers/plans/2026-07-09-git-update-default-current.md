# git-update-default 현재 브랜치 모드 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `git-update-default` 에 `--current` 플래그를 추가해, 브랜치 전환 없이 현재 체크아웃된 브랜치를 그 브랜치의 upstream 까지 fast-forward 하는 모드를 제공한다.

**Architecture:** 기존 도구는 default branch 를 해석해 그 브랜치로 전환한 뒤 `origin/<default>` 로 fast-forward 한다. 여기에 모드 분기를 하나 더한다. `--current` 이면 default 해석·전환을 건너뛰고, 현재 브랜치의 upstream(`@{upstream}`)을 해석해 그 ref 로 fast-forward 만 한다. fetch·dirty 처리·divergence 경고 같은 공유 로직은 그대로 재사용한다.

**Tech Stack:** Go(표준 라이브러리만), git CLI 호출, 기존 `internal/gitx`·`internal/resolve`·`internal/confirm` 패키지.

## Global Constraints

- Go 모듈 baseline 은 `go 1.23` 을 유지한다. 새 의존성을 추가하지 않는다(표준 라이브러리만 사용).
- 커밋은 Conventional Commits 형식을 따른다(`feat`/`test`/`docs` 등). 헤더 100자 이내.
- 모든 사용자 대면 출력·주석·문서는 한국어로 작성한다.
- 기존 default 모드(무인자·`--stash`·`--force`) 동작을 한 글자도 바꾸지 않는다(하위호환).
- 릴리스 배선(`.github/workflows/`, `.goreleaser.yaml`, `release-please-config.json`, `.release-please-manifest.json`, homebrew formula)은 건드리지 않는다.
- 작업 디렉터리는 `apps/git-update-default/`. 모든 상대 경로는 이 디렉터리 기준이다. 테스트·빌드는 그 안에서 `mise run test` / `mise run build` 로 실행한다.

## File Structure

- `internal/gitx/gitx.go` — 현재 브랜치의 upstream 을 해석하는 `Upstream()` 과 임의 ref 로 ff 하는 `MergeFFOnlyRef()` 를 더한다.
- `internal/gitx/gitx_test.go` — 위 두 함수를 실제 임시 git 저장소로 검증한다(기존 `gitInit` 헬퍼 재사용).
- `cmd/git-update-default/main.go` — `options.current` 필드, `parseArgs` 의 `--current`, `helpText`, `run()` 의 모드 분기와 `runCurrent()` 를 더한다.
- `cmd/git-update-default/main_test.go` — `parseArgs` 가 `--current` 를 파싱하는지 단위 테스트한다.
- `completions/_git-update-default`, `completions/git-update-default.bash` — `--current` 후보를 더한다.
- `README.md` — 동작·사용 절에 `--current` 를 반영한다.

---

### Task 1: gitx 에 Upstream() 과 MergeFFOnlyRef() 추가

현재 브랜치의 upstream ref 이름을 해석하는 함수와, 임의 ref 로 fast-forward 하는 함수를 더한다. 기존 `MergeFFOnly(name)` 은 새 `MergeFFOnlyRef` 를 호출하도록 리팩터해 동작을 그대로 유지한다.

**Files:**
- Modify: `internal/gitx/gitx.go` (끝의 `MergeFFOnly` 근처)
- Test: `internal/gitx/gitx_test.go` (파일 끝에 추가)

**Interfaces:**
- Produces:
  - `func Upstream() (string, error)` — 현재 브랜치의 upstream 추적 ref 이름(예: `"origin/main"`)을 돌려준다. upstream 미설정이면 에러.
  - `func MergeFFOnlyRef(ref string) error` — 현재 브랜치를 `ref` 까지 `merge --ff-only` 한다.
  - `func MergeFFOnly(name string) error` — 기존과 동일하게 `origin/<name>` 으로 ff(내부적으로 `MergeFFOnlyRef` 호출).

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/gitx/gitx_test.go` 끝에 아래를 추가한다. `gitInit` 은 파일 상단에 이미 있는 헬퍼로, 커밋 하나가 있는 임시 저장소를 만들고 그 안으로 cwd 를 옮긴다.

```go
func TestUpstreamAndMergeFFOnlyRef(t *testing.T) {
	// origin 역할을 할 bare 저장소를 만들고, gitInit 저장소를 그 origin 에 연결한다.
	upstreamDir := t.TempDir()
	for _, args := range [][]string{{"init", "--bare", "-b", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = upstreamDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	gitInit(t) // cwd = 커밋 하나 있는 작업 저장소
	mustGit(t, "remote", "add", "origin", upstreamDir)
	mustGit(t, "push", "-u", "origin", "main")

	// upstream 이 설정됐으니 Upstream 은 origin/main 을 돌려준다.
	up, err := Upstream()
	if err != nil {
		t.Fatalf("Upstream err = %v, want nil", err)
	}
	if up != "origin/main" {
		t.Fatalf("Upstream = %q, want origin/main", up)
	}

	// origin 을 한 커밋 앞서게 만들고(다른 클론에서 push), fetch 후 ff 되는지 본다.
	other := t.TempDir()
	mustGitDir(t, other, "clone", upstreamDir, ".")
	if err := os.WriteFile(filepath.Join(other, "g.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitDir(t, other, "add", ".")
	mustGitDir(t, other, "commit", "-m", "second")
	mustGitDir(t, other, "push", "origin", "main")

	if err := FetchPrune(); err != nil {
		t.Fatalf("FetchPrune err = %v", err)
	}
	if err := MergeFFOnlyRef(up); err != nil {
		t.Fatalf("MergeFFOnlyRef(%q) err = %v, want nil", up, err)
	}
	// ff 후 g.txt 가 존재해야 한다.
	if _, err := os.Stat("g.txt"); err != nil {
		t.Fatalf("ff 후 g.txt 없음: %v", err)
	}

	// upstream 이 없는 브랜치로 옮기면 Upstream 은 에러.
	mustGit(t, "switch", "-c", "no-upstream")
	if _, err := Upstream(); err == nil {
		t.Fatal("Upstream on branch without upstream = nil err, want error")
	}
}

// mustGit 은 현재 cwd 에서 git 을 실행하고 실패 시 종료한다.
func mustGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// mustGitDir 은 지정한 디렉터리에서 git 을 실행하고 실패 시 종료한다.
func mustGitDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git -C %s %v: %v: %s", dir, args, err, out)
	}
}
```

- [ ] **Step 2: 테스트를 실행해 실패를 확인**

Run: `cd apps/git-update-default && go test ./internal/gitx/ -run TestUpstreamAndMergeFFOnlyRef -v`
Expected: 컴파일 실패가 아니라 `undefined: Upstream` / `undefined: MergeFFOnlyRef` 로 FAIL (아직 함수가 없음). 함수 시그니처만 먼저 빈 구현으로 넣어 실행 실패를 관찰하고 싶으면, 아래 빈 골격을 먼저 넣고 실행해 `Upstream = "", want origin/main` 같은 값 불일치 FAIL 을 눈으로 확인한다.

```go
func Upstream() (string, error)              { return "", nil }
func MergeFFOnlyRef(ref string) error        { return nil }
```

- [ ] **Step 3: 최소 구현 작성**

`internal/gitx/gitx.go` 의 기존 `MergeFFOnly` 를 아래로 교체하고, `Upstream` 을 그 근처에 추가한다.

```go
// Upstream 은 현재 브랜치가 추적하는 upstream ref 이름을 돌려준다(예: "origin/main").
// upstream 이 설정돼 있지 않으면 에러를 돌려준다. 추측하지 않는다.
func Upstream() (string, error) {
	out, err := run("rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", fmt.Errorf("upstream 이 설정되지 않았습니다")
	}
	return name, nil
}

// MergeFFOnlyRef 은 현재 브랜치를 임의의 ref 까지 fast-forward 한다.
// 갈라져서 fast-forward 가 불가능하면 에러를 돌려준다(강제하지 않는다).
func MergeFFOnlyRef(ref string) error {
	_, err := run("merge", "--ff-only", ref)
	return err
}

// MergeFFOnly 는 현재 브랜치를 origin/<name> 까지 fast-forward 한다.
func MergeFFOnly(name string) error {
	return MergeFFOnlyRef("origin/" + name)
}
```

- [ ] **Step 4: 테스트를 실행해 통과를 확인**

Run: `cd apps/git-update-default && go test ./internal/gitx/ -v`
Expected: `TestUpstreamAndMergeFFOnlyRef` 을 포함해 gitx 패키지 전원 PASS.

- [ ] **Step 5: 커밋**

```bash
git add apps/git-update-default/internal/gitx/gitx.go apps/git-update-default/internal/gitx/gitx_test.go
git commit -m "feat(git-update-default): gitx 에 Upstream·MergeFFOnlyRef 추가"
```

---

### Task 2: parseArgs 에 --current 플래그 추가

`options` 에 `current` 필드를 더하고 `parseArgs` 가 `--current` 를 파싱하게 한다.

**Files:**
- Modify: `cmd/git-update-default/main.go:58-76` (`options` 구조체와 `parseArgs`)
- Test: `cmd/git-update-default/main_test.go:13-37` (`TestParseArgs`)

**Interfaces:**
- Consumes: 없음
- Produces: `options.current bool` — `--current` 가 주어졌는지.

- [ ] **Step 1: 실패하는 테스트 작성**

`main_test.go` 의 `TestParseArgs` 를 아래로 교체한다(케이스에 `wantCurrent` 추가).

```go
func TestParseArgs(t *testing.T) {
	cases := []struct {
		args        []string
		wantStash   bool
		wantForce   bool
		wantCurrent bool
		wantErr     bool
	}{
		{nil, false, false, false, false},
		{[]string{"--stash"}, true, false, false, false},
		{[]string{"--force"}, false, true, false, false},
		{[]string{"--current"}, false, false, true, false},
		{[]string{"--current", "--stash"}, true, false, true, false},
		{[]string{"--bogus"}, false, false, false, true},
	}
	for _, c := range cases {
		o, err := parseArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Fatalf("parseArgs(%v) err=%v wantErr=%v", c.args, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if o.stash != c.wantStash || o.force != c.wantForce || o.current != c.wantCurrent {
			t.Fatalf("parseArgs(%v) = %+v", c.args, o)
		}
	}
}
```

- [ ] **Step 2: 테스트를 실행해 실패를 확인**

Run: `cd apps/git-update-default && go test ./cmd/... -run TestParseArgs -v`
Expected: `o.current` 미정의로 컴파일 실패. 값 불일치 FAIL 을 눈으로 보려면 `options` 에 `current bool` 필드만 먼저 더하고 `parseArgs` 는 그대로 둔 채 실행해, `--current` 케이스가 `wantCurrent=true` 와 어긋나 FAIL 하는 것을 확인한다.

- [ ] **Step 3: 최소 구현 작성**

`main.go` 의 `options` 와 `parseArgs` 를 수정한다.

```go
type options struct {
	stash   bool
	force   bool
	current bool
}

func parseArgs(args []string) (options, error) {
	o := options{}
	for _, a := range args {
		switch a {
		case "--stash":
			o.stash = true
		case "--force":
			o.force = true
		case "--current":
			o.current = true
		default:
			return o, fmt.Errorf("알 수 없는 옵션: %s", a)
		}
	}
	return o, nil
}
```

- [ ] **Step 4: 테스트를 실행해 통과를 확인**

Run: `cd apps/git-update-default && go test ./cmd/... -v`
Expected: `TestParseArgs` 포함 전원 PASS.

- [ ] **Step 5: 커밋**

```bash
git add apps/git-update-default/cmd/git-update-default/main.go apps/git-update-default/cmd/git-update-default/main_test.go
git commit -m "feat(git-update-default): parseArgs 에 --current 플래그 추가"
```

---

### Task 3: run() 모드 분기와 runCurrent() 구현 + 도움말 갱신

`run()` 에서 fetch 직후 `opts.current` 이면 `runCurrent()` 로 분기한다. `runCurrent()` 는 현재 브랜치·upstream 을 해석하고, dirty 처리를 공유한 뒤 upstream 으로 ff 한다. `helpText` 에 `--current` 설명을 더한다. 이 태스크는 실제 저장소 E2E 스모크로 종결한다.

**Files:**
- Modify: `cmd/git-update-default/main.go` (`helpText`, `run()` 에 분기 삽입, `runCurrent()` 신규 함수)

**Interfaces:**
- Consumes: `options.current`(Task 2), `gitx.Upstream()`·`gitx.MergeFFOnlyRef()`(Task 1), 기존 `gitx.CurrentBranch`·`gitx.DirtyFiles`, 기존 `handleDirty`·`dirtyProceed`/`dirtyAbortOK`/`dirtyAbortErr`.
- Produces: `func runCurrent(opts options) int`

- [ ] **Step 1: helpText 에 --current 추가**

`main.go` 의 `helpText` 상수에서 옵션 목록에 한 줄을 더한다(`--stash` 위, 또는 `--force` 아래 자연스러운 위치).

```go
const helpText = `Usage: git-update-default [--current] [--stash | --force]

현재 위치가 속한 git 저장소를 원격 default branch 의 최신 상태로 전환한다.
저장소 안 어느 하위 경로에서 실행해도 동작한다.

  git-update-default          default branch 로 전환하고 최신까지 fast-forward
  --current                   전환 없이 현재 브랜치를 그 브랜치의 upstream 까지 fast-forward
  --stash                     dirty 일 때 묻지 않고 stash 후 진행
  --force                     dirty 일 때 묻지 않고 추적 변경을 버리고 진행
  -v, --version               버전 출력
  -h, --help                  도움말 출력

dirty 인 채로 인터랙티브하게 실행하면 변경 파일 목록을 보여주고
stash 후 진행 / force / 취소(기본값) 를 고를 수 있다.
`
```

- [ ] **Step 2: run() 에 모드 분기 삽입**

`main.go` 의 `run()` 에서 `gitx.FetchPrune()` 블록 바로 다음(기존 `resolve.Default` 호출 직전)에 아래 분기를 넣는다.

```go
	if opts.current {
		return runCurrent(opts)
	}
```

- [ ] **Step 3: runCurrent() 구현**

`run()` 아래에 새 함수를 추가한다.

```go
// runCurrent 는 --current 모드 본체다. 브랜치 전환 없이 현재 브랜치를 그 브랜치의
// upstream 까지 fast-forward 한다. 이 함수는 run() 이 IsRepo·HasOriginRemote·
// FetchPrune 을 이미 마친 뒤에만 호출된다.
func runCurrent(opts options) int {
	branch := gitx.CurrentBranch()
	if branch == "" {
		_, _ = fmt.Fprintln(os.Stderr, "git-update-default: detached HEAD 상태라 현재 브랜치를 갱신할 수 없습니다.")
		return 1
	}
	upstream, err := gitx.Upstream()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "git-update-default: %s 에 upstream 이 설정되지 않았습니다.\n", branch)
		_, _ = fmt.Fprintln(os.Stderr, "  → `git push -u origin <브랜치>` 로 upstream 을 설정한 뒤 다시 실행하세요.")
		return 1
	}

	files, err := gitx.DirtyFiles()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "git-update-default:", err)
		return 1
	}
	if len(files) > 0 {
		switch handleDirty(files, opts) {
		case dirtyAbortOK:
			return 0
		case dirtyAbortErr:
			return 1
		}
	}

	if err := gitx.MergeFFOnlyRef(upstream); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "git-update-default: %s 가 %s 와 갈라져 fast-forward 할 수 없습니다.\n", branch, upstream)
		_, _ = fmt.Fprintln(os.Stderr, "  → 직접 rebase·reset 으로 정리하세요. 강제로 맞추지 않습니다.")
		return 1
	}

	_, _ = fmt.Printf("✓ %s 를 %s 최신까지 맞췄습니다.\n", branch, upstream)
	return 0
}
```

- [ ] **Step 4: 단위 테스트·빌드 확인**

Run: `cd apps/git-update-default && go test ./... && go build ./...`
Expected: 전원 PASS, 빌드 성공.

- [ ] **Step 5: E2E 스모크 (실제 git 저장소, 1회성)**

아래 스크립트를 세션 전용 임시 디렉터리에서 실행해 세 경로를 검증한다: (a) upstream 이 있는 브랜치에서 `--current` 가 ff 로 성공, (b) upstream 이 없는 브랜치에서 종료 코드 1, (c) default 모드가 여전히 정상.

```bash
cd apps/git-update-default
go build -o /tmp/gud ./cmd/git-update-default

WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}/gud-e2e"
rm -rf "$WORK"; mkdir -p "$WORK"; cd "$WORK"

git init --bare -b main origin.git
git clone origin.git work; cd work
git config user.email test@example.com; git config user.name test
echo a > a.txt; git add .; git commit -m init; git push -u origin main

# feature 브랜치를 만들어 origin 에 push (upstream 설정)
git switch -c feature; git push -u origin feature

# 다른 클론에서 origin/feature 를 한 커밋 앞서게 만든다
cd "$WORK"; git clone origin.git other; cd other
git switch feature; echo b > b.txt; git add .; git commit -m second; git push origin feature

# (a) work 로 돌아와 --current 실행 → feature 가 origin/feature 로 ff, b.txt 생김
cd "$WORK/work"; git switch feature
/tmp/gud --current; echo "EXIT(a)=$?"
test -f b.txt && echo "OK: b.txt ff 됨" || echo "FAIL: ff 안 됨"

# (b) upstream 없는 로컬 브랜치에서 --current → 종료 코드 1
git switch -c local-only
/tmp/gud --current; echo "EXIT(b)=$? (기대 1)"

# (c) default 모드는 그대로: main 으로 전환+ff
/tmp/gud; echo "EXIT(c)=$? (기대 0)"
git branch --show-current  # main 이어야 함
```

Expected: `EXIT(a)=0` 과 `OK: b.txt ff 됨`, `EXIT(b)=1`, `EXIT(c)=0` 이고 마지막 브랜치가 `main`. 종료 코드와 출력을 완료 보고에 증거로 남긴다. 검증 후 `/tmp/gud` 와 `$WORK` 를 지운다.

- [ ] **Step 6: 커밋**

```bash
git add apps/git-update-default/cmd/git-update-default/main.go
git commit -m "feat(git-update-default): --current 모드 구현"
```

---

### Task 4: 완성 파일과 README 갱신

셸 자동완성 후보와 README 에 `--current` 를 반영한다.

**Files:**
- Modify: `completions/_git-update-default`
- Modify: `completions/git-update-default.bash`
- Modify: `README.md`

**Interfaces:**
- Consumes: `--current` 플래그(Task 2·3)
- Produces: 없음(문서·완성)

- [ ] **Step 1: zsh 완성 갱신**

`completions/_git-update-default` 의 `_arguments` 목록 첫 줄에 `--current` 를 더한다.

```zsh
#compdef git-update-default

_arguments \
  '--current[전환 없이 현재 브랜치를 upstream 까지 fast-forward]' \
  '--stash[dirty 일 때 묻지 않고 stash 후 진행]' \
  '--force[dirty 일 때 묻지 않고 추적 변경을 버리고 진행]' \
  '--version[버전 출력]' \
  '--help[도움말 출력]'
```

- [ ] **Step 2: bash 완성 갱신**

`completions/git-update-default.bash` 의 `opts` 에 `--current` 를 더한다.

```bash
_git_update_default() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--current --stash --force --version --help"
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_update_default git-update-default
```

- [ ] **Step 3: README 갱신**

`README.md` 의 "동작" 절에 현재 브랜치 모드를 한 항목으로 설명하고, "사용" 절 예시에 `--current` 를 더한다. 아래처럼 동작 절 끝에 문장을 더하고, 사용 절 코드블록에 한 줄을 추가한다.

동작 절(기존 5개 항목 아래에 추가):

```markdown
`--current` 를 주면 default branch 로 전환하지 않고, 지금 체크아웃된 브랜치를
그 브랜치의 upstream(`@{upstream}`)까지 fast-forward 한다. upstream 이 없거나
detached HEAD 이면 아무것도 바꾸지 않고 멈춘다.
```

사용 절 코드블록:

```
    git update-default            # 또는 git-update-default
    git update-default --current  # 전환 없이 현재 브랜치를 upstream 까지 fast-forward
    git update-default --stash    # dirty 일 때 묻지 않고 stash 후 진행
    git update-default --force    # dirty 일 때 묻지 않고 추적 변경 폐기 후 진행
```

- [ ] **Step 4: 완성 파일 문법 확인**

Run: `zsh -n apps/git-update-default/completions/_git-update-default; bash -n apps/git-update-default/completions/git-update-default.bash; echo OK`
Expected: 문법 오류 없이 `OK`.

- [ ] **Step 5: 커밋**

```bash
git add apps/git-update-default/completions/ apps/git-update-default/README.md
git commit -m "docs(git-update-default): --current 완성·README 반영"
```

---

## Self-Review

**1. Spec coverage:**
- 모드 분기(default 유지, current 추가) → Task 3.
- 현재 브랜치 upstream 해석 → Task 1(`Upstream`) + Task 3.
- detached HEAD 에러 → Task 3 `runCurrent` 첫 분기.
- upstream 미설정 에러(추측 안 함) → Task 1(에러 반환) + Task 3.
- fetch best-effort → 기존 `run()` 의 `FetchPrune` 그대로, current 분기가 그 뒤에 옴(Task 3 Step 2).
- dirty 처리 재사용·`--current` 와 `--stash`/`--force` 조합 → Task 2(파싱) + Task 3(`handleDirty` 공유).
- divergence 경고 후 멈춤 → Task 3 `MergeFFOnlyRef` 에러 경로.
- 완성·README → Task 4.
- 수용 기준·검증 방식 → 단위(Task 1·2) + E2E 스모크(Task 3 Step 5).

**2. Placeholder scan:** 모든 코드 블록은 실제 내용. TBD/TODO 없음.

**3. Type consistency:** `Upstream() (string, error)`, `MergeFFOnlyRef(ref string) error`, `options.current`, `runCurrent(opts options) int` 이 Task 간 일관. `handleDirty`·`dirtyAbortOK`/`dirtyAbortErr` 는 기존 코드의 것을 그대로 참조.
