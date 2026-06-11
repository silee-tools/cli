# git-update-default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git 저장소 안 어느 하위 경로에서 실행하든 그 저장소를 원격 default branch(main → master → gh → origin/HEAD 순으로 탐색)의 최신 상태로 전환하는 Go CLI 도구를 `apps/git-update-default/` 에 추가한다.

**Architecture:** 형제 도구 git-tidy 와 같은 구조다. 진입점 `cmd/git-update-default/main.go` 가 인자 파싱·버전·오케스트레이션을 맡고, `internal/gitx` 가 git·gh CLI 호출을 얇게 감싸며, 핵심 판단 로직(default branch 우선순위 탐색, dirty 처리 경로 결정, TUI 상태 전이)은 git 호출에 의존하지 않는 순수 함수로 분리해 단위 테스트한다. dirty 일 때의 3지선다(stash/force/취소)는 bubbletea 단일 선택 TUI 로 받는다.

**Tech Stack:** Go 1.23, bubbletea v1.3.4 + lipgloss v1.0.0(git-tidy 와 동일 핀), golang.org/x/term, 외부 `git`·선택적 `gh` CLI.

**기준 문서:** 이 계획의 단일 설계 기준은 `docs/superpowers/specs/2026-06-11-git-update-default-design.md` 다. 완료 검토는 계획서가 아니라 이 spec 에 직접 대조한다.

---

## 파일 구조

생성할 파일과 각 책임:

- `apps/git-update-default/cmd/git-update-default/main.go` — 진입점. 인자 파싱(`parseArgs`), `-v`/`-h`, `versionLine`, `run()` 오케스트레이션, dirty 경로 분기(`dirtyPath`).
- `apps/git-update-default/cmd/git-update-default/main_test.go` — `versionLine`·`parseArgs`·`dirtyPath` 순수 함수 테스트.
- `apps/git-update-default/internal/gitx/gitx.go` — git CLI 호출 래퍼(저장소·원격·브랜치·dirty·stash·reset·switch·merge).
- `apps/git-update-default/internal/gitx/gh.go` — gh CLI 로 GitHub default branch 조회.
- `apps/git-update-default/internal/resolve/resolve.go` — default branch 우선순위 탐색(순수, 함수 주입).
- `apps/git-update-default/internal/resolve/resolve_test.go` — 우선순위 로직 테스트.
- `apps/git-update-default/internal/confirm/confirm.go` — TTY 감지, Action 타입.
- `apps/git-update-default/internal/confirm/tui.go` — bubbletea 단일 선택 TUI.
- `apps/git-update-default/internal/confirm/confirm_test.go` — TUI 상태 전이(`Update`) 순수 테스트.
- `apps/git-update-default/go.mod` / `go.sum` — 모듈 정의(go 1.23 baseline 유지).
- `apps/git-update-default/.mise.toml` — build/test/lint/fmt/install 태스크.
- `apps/git-update-default/.goreleaser.yaml` — darwin/linux × amd64/arm64 빌드.
- `apps/git-update-default/.gitignore` — 빌드 산출물 무시.
- `apps/git-update-default/README.md` — 도구 설명.
- `apps/git-update-default/completions/_git-update-default` / `git-update-default.bash` — 자동완성.
- `.github/workflows/git-update-default-ci.yml` — CI(fmt-check → lint → test → build).

수정할 파일:

- `release-please-config.json` — packages 에 새 도구 추가.
- `.release-please-manifest.json` — 새 도구 `0.0.0` 추가.
- `README.md`(루트) / `docs/README_ko.md` — 도구 표에 새 도구 추가.

별도 저장소(이 계획 범위 밖, 후속): `silee-tools/homebrew-tap` 의 `Formula/git-update-default.rb` 골격.

---

## Task 1: 프로젝트 스캐폴드와 버전 플래그

도구를 빌드 가능한 최소 상태로 세우고, 버전 형식 적합성 게이트를 통과시킨다.

**Files:**
- Create: `apps/git-update-default/go.mod`
- Create: `apps/git-update-default/.gitignore`
- Create: `apps/git-update-default/.mise.toml`
- Create: `apps/git-update-default/.goreleaser.yaml`
- Create: `apps/git-update-default/cmd/git-update-default/main.go`
- Test: `apps/git-update-default/cmd/git-update-default/main_test.go`

- [ ] **Step 1: go.mod 작성**

`apps/git-update-default/go.mod`:

```
module github.com/silee-tools/git-update-default

go 1.23

require (
	github.com/charmbracelet/bubbletea v1.3.4
	github.com/charmbracelet/lipgloss v1.0.0
	golang.org/x/term v0.20.0
)
```

indirect 블록은 `go mod tidy` 가 채운다. git-tidy 가 같은 버전을 쓰므로 `go 1.23` 이 1.24 로 올라가지 않는다.

- [ ] **Step 2: 보조 설정 파일 작성**

`apps/git-update-default/.gitignore`:

```
/git-update-default
```

`apps/git-update-default/.mise.toml`:

```toml
[tools]
go = "1.23"
"aqua:golangci/golangci-lint" = "latest"

[tasks.build]
description = "Build the project"
run = "go build ./cmd/git-update-default"

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
run = "go build -o ~/.local/bin/git-update-default ./cmd/git-update-default"
```

`apps/git-update-default/.goreleaser.yaml`:

```yaml
version: 2

project_name: git-update-default

builds:
  - main: ./cmd/git-update-default
    binary: git-update-default
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

- [ ] **Step 3: 버전 플래그 실패 테스트 작성**

`apps/git-update-default/cmd/git-update-default/main_test.go`:

```go
package main

import "testing"

func TestVersionLine(t *testing.T) {
	got := versionLine("git-update-default", "1.2.3")
	want := "git-update-default v1.2.3 © 2026 silee-tools\n"
	if got != want {
		t.Fatalf("versionLine = %q, want %q", got, want)
	}
}
```

- [ ] **Step 4: 테스트 실패 확인**

Run: `cd apps/git-update-default && go test ./cmd/... 2>&1 | head`
Expected: 컴파일 실패(`undefined: versionLine`).

- [ ] **Step 5: main.go 최소 구현**

`apps/git-update-default/cmd/git-update-default/main.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var version = "dev"

func versionLine(name, version string) string {
	return fmt.Sprintf("%s v%s © 2026 silee-tools\n", name, version)
}

const helpText = `Usage: git-update-default [--stash | --force]

현재 위치가 속한 git 저장소를 원격 default branch 의 최신 상태로 전환한다.
저장소 안 어느 하위 경로에서 실행해도 동작한다.

  git-update-default          default branch 로 전환하고 최신까지 fast-forward
  --stash                     dirty 일 때 묻지 않고 stash 후 진행
  --force                     dirty 일 때 묻지 않고 추적 변경을 버리고 진행
  -v, --version               버전 출력
  -h, --help                  도움말 출력

dirty 인 채로 인터랙티브하게 실행하면 변경 파일 목록을 보여주고
stash 후 진행 / force / 취소(기본값) 를 고를 수 있다.
`

func main() {
	invoked := filepath.Base(os.Args[0])
	args := os.Args[1:]

	for _, a := range args {
		switch a {
		case "-v", "--version":
			_, _ = fmt.Fprint(os.Stdout, versionLine(invoked, version))
			return
		case "-h", "--help":
			fmt.Print(helpText)
			return
		}
	}

	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-update-default:", err)
		fmt.Fprintln(os.Stderr, "git-update-default --help 로 사용법을 확인하세요.")
		os.Exit(1)
	}
	os.Exit(run(opts))
}
```

`parseArgs`·`run` 본구현은 Task 5 에서 완성한다. 이 Task 를 빌드·테스트 가능하게 두기 위해, 같은 파일 아래에 임시 스텁을 둔다(Task 5 에서 교체):

```go
type options struct {
	stash bool
	force bool
}

func parseArgs(args []string) (options, error) {
	o := options{}
	for _, a := range args {
		switch a {
		case "--stash":
			o.stash = true
		case "--force":
			o.force = true
		default:
			return o, fmt.Errorf("알 수 없는 옵션: %s", a)
		}
	}
	return o, nil
}

func run(opts options) int {
	_ = opts
	fmt.Fprintln(os.Stderr, "git-update-default: 아직 구현되지 않았습니다.")
	return 1
}
```

- [ ] **Step 6: 테스트 통과 + 빌드 확인**

Run: `cd apps/git-update-default && go mod tidy && go test ./cmd/... && go build ./cmd/git-update-default && ./git-update-default -v`
Expected: 테스트 PASS, 빌드 성공, 마지막 줄 `git-update-default vdev © 2026 silee-tools`.

- [ ] **Step 7: 커밋**

```bash
git add apps/git-update-default/go.mod apps/git-update-default/go.sum apps/git-update-default/.gitignore apps/git-update-default/.mise.toml apps/git-update-default/.goreleaser.yaml apps/git-update-default/cmd
git commit -m "feat(git-update-default): 스캐폴드와 버전 플래그"
```

---

## Task 2: gitx 패키지 — git·gh CLI 호출 래퍼

저장소 상태 조회와 조작에 필요한 git 호출, gh 의 default branch 조회를 얇게 감싼다. 외부 명령에 의존하므로 임시 저장소를 만들어 통합 테스트로 핵심 경로를 검증한다.

**Files:**
- Create: `apps/git-update-default/internal/gitx/gitx.go`
- Create: `apps/git-update-default/internal/gitx/gh.go`
- Test: `apps/git-update-default/internal/gitx/gitx_test.go`

- [ ] **Step 1: gitx.go 작성**

`apps/git-update-default/internal/gitx/gitx.go`:

```go
// Package gitx 는 git CLI 를 호출해 git-update-default 가 쓰는 정보를 모으고
// 저장소를 조작한다.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// run 은 git 을 실행하고 표준 출력을 돌려준다. 실패 시 git 의 stderr 를 에러에
// 포함해, 호출자가 불투명한 "exit status N" 대신 실제 원인을 볼 수 있게 한다.
func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
		}
	}
	return stdout.String(), err
}

// IsRepo 는 현재 디렉터리가 git 작업 트리 안인지 본다. 하위 경로에서 실행해도
// git 이 상위로 거슬러 저장소를 찾으므로, 별도의 root 탐색 로직이 필요 없다.
func IsRepo() bool {
	out, err := run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// HasOriginRemote 는 origin 원격이 설정돼 있는지 본다.
func HasOriginRemote() bool {
	_, err := run("remote", "get-url", "origin")
	return err == nil
}

// FetchPrune 는 origin 의 최신 상태를 받고 사라진 원격 브랜치를 정리한다.
// 실패(오프라인 등)는 에러로 돌려, 호출자가 경고할지 정한다.
func FetchPrune() error {
	_, err := run("fetch", "origin", "--prune")
	return err
}

// RemoteBranchExists 는 origin/<name> 원격 추적 ref 가 있는지 본다.
func RemoteBranchExists(name string) bool {
	_, err := run("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+name)
	return err == nil
}

// SymbolicRefDefault 는 origin/HEAD 가 가리키는 default branch 이름을 돌려준다.
// refs/remotes/origin/HEAD 가 없으면 set-head 로 한 번 갱신을 시도한 뒤 다시 읽는다.
func SymbolicRefDefault() (string, bool) {
	read := func() (string, bool) {
		out, err := run("symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return "", false
		}
		name := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
		return name, name != ""
	}
	if name, ok := read(); ok {
		return name, true
	}
	_, _ = run("remote", "set-head", "origin", "-a")
	return read()
}

// CurrentBranch 는 체크아웃된 브랜치 이름을 돌려준다(detached 면 빈 문자열).
func CurrentBranch() string {
	out, _ := run("branch", "--show-current")
	return strings.TrimSpace(out)
}

// DirtyFiles 는 커밋되지 않은 변경 파일을 git status --porcelain 형식 줄로
// 돌려준다(추적되지 않는 파일 포함). 비어 있으면 작업 트리가 clean 이다.
func DirtyFiles() ([]string, error) {
	out, err := run("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// StashPush 는 추적되지 않는 파일까지 포함해 작업 트리 변경을 보관한다.
func StashPush() error {
	msg := "git-update-default " + time.Now().Format(time.RFC3339)
	_, err := run("stash", "push", "-u", "-m", msg)
	return err
}

// ResetHard 는 추적되는 파일의 커밋되지 않은 변경을 버린다. 추적되지 않는
// 새 파일은 건드리지 않는다.
func ResetHard() error {
	_, err := run("reset", "--hard", "HEAD")
	return err
}

// LocalBranchExists 는 로컬에 <name> 브랜치가 있는지 본다.
func LocalBranchExists(name string) bool {
	_, err := run("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Switch 는 로컬 브랜치로 전환한다.
func Switch(name string) error {
	_, err := run("switch", name)
	return err
}

// SwitchCreateTracking 은 origin/<name> 을 추적하는 로컬 브랜치를 만들어 전환한다.
func SwitchCreateTracking(name string) error {
	_, err := run("switch", "-c", name, "--track", "origin/"+name)
	return err
}

// MergeFFOnly 는 현재 브랜치를 origin/<name> 까지 fast-forward 한다.
// 갈라져서 fast-forward 가 불가능하면 에러를 돌려준다(강제하지 않는다).
func MergeFFOnly(name string) error {
	_, err := run("merge", "--ff-only", "origin/"+name)
	return err
}
```

- [ ] **Step 2: gh.go 작성**

`apps/git-update-default/internal/gitx/gh.go`:

```go
package gitx

import (
	"bytes"
	"os/exec"
	"strings"
)

// GitHubDefault 는 gh CLI 로 현재 저장소의 GitHub default branch 를 조회한다.
// gh 가 없거나, 인증되지 않았거나, GitHub 저장소가 아니면 ok=false 를 돌려준다.
func GitHubDefault() (string, bool) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", false
	}
	cmd := exec.Command("gh", "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	name := strings.TrimSpace(stdout.String())
	return name, name != ""
}
```

- [ ] **Step 3: 통합 테스트 작성**

`apps/git-update-default/internal/gitx/gitx_test.go`:

```go
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInit 은 임시 디렉터리에 커밋 하나가 있는 git 저장소를 만들고, 그 안으로
// 작업 디렉터리를 옮긴다. 테스트가 끝나면 원래 디렉터리로 돌아간다.
func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	steps := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func TestIsRepoAndDirty(t *testing.T) {
	gitInit(t)

	if !IsRepo() {
		t.Fatal("IsRepo = false, want true in a git repo")
	}

	files, err := DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("clean repo DirtyFiles = %v, want empty", files)
	}

	if err := os.WriteFile("f.txt", []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err = DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("dirty repo DirtyFiles = %v, want 1 entry", files)
	}
}

func TestIsRepoFalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if IsRepo() {
		t.Fatal("IsRepo = true outside a repo, want false")
	}
}
```

- [ ] **Step 4: 테스트 실행(통과 확인)**

Run: `cd apps/git-update-default && go test ./internal/gitx/... -v`
Expected: `TestIsRepoAndDirty`, `TestIsRepoFalseOutsideRepo` PASS. 이 통합 테스트는 래퍼가 실제 git 출력을 올바로 해석하는지(IsRepo 의 `--is-inside-work-tree`, DirtyFiles 의 porcelain 파싱) 검증한다.

- [ ] **Step 5: 커밋**

```bash
git add apps/git-update-default/internal/gitx
git commit -m "feat(git-update-default): gitx·gh CLI 호출 래퍼"
```

---

## Task 3: resolve 패키지 — default branch 우선순위 탐색

main → master → gh → origin/HEAD 순서로 default branch 를 정하는 순수 로직을 함수 주입으로 분리해 테스트한다.

**Files:**
- Create: `apps/git-update-default/internal/resolve/resolve.go`
- Test: `apps/git-update-default/internal/resolve/resolve_test.go`

- [ ] **Step 1: 실패 테스트 작성**

`apps/git-update-default/internal/resolve/resolve_test.go`:

```go
package resolve

import "testing"

func deps(remote map[string]bool, gh string, symref string) Deps {
	return Deps{
		RemoteBranchExists: func(n string) bool { return remote[n] },
		GitHubDefault:      func() (string, bool) { return gh, gh != "" },
		SymbolicRef:        func() (string, bool) { return symref, symref != "" },
	}
}

func TestDefaultPrefersMain(t *testing.T) {
	got, ok := Default(deps(map[string]bool{"main": true, "master": true}, "develop", "trunk"))
	if !ok || got != "main" {
		t.Fatalf("Default = %q,%v want main,true", got, ok)
	}
}

func TestDefaultFallsToMaster(t *testing.T) {
	got, ok := Default(deps(map[string]bool{"master": true}, "develop", "trunk"))
	if !ok || got != "master" {
		t.Fatalf("Default = %q,%v want master,true", got, ok)
	}
}

func TestDefaultFallsToGitHub(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "develop", "trunk"))
	if !ok || got != "develop" {
		t.Fatalf("Default = %q,%v want develop,true", got, ok)
	}
}

func TestDefaultFallsToSymbolicRef(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "", "trunk"))
	if !ok || got != "trunk" {
		t.Fatalf("Default = %q,%v want trunk,true", got, ok)
	}
}

func TestDefaultUnresolved(t *testing.T) {
	got, ok := Default(deps(map[string]bool{}, "", ""))
	if ok || got != "" {
		t.Fatalf("Default = %q,%v want \"\",false", got, ok)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-update-default && go test ./internal/resolve/... 2>&1 | head`
Expected: 컴파일 실패(`undefined: Default`, `undefined: Deps`).

- [ ] **Step 3: resolve.go 구현**

`apps/git-update-default/internal/resolve/resolve.go`:

```go
// Package resolve 는 원격 default branch 이름을 우선순위에 따라 정한다.
package resolve

// Deps 는 탐색에 필요한 외부 조회를 함수로 주입받는다. 이렇게 분리해 우선순위
// 로직을 git·gh 호출 없이 순수 함수로 테스트한다.
type Deps struct {
	RemoteBranchExists func(name string) bool // origin/<name> 원격 추적 ref 존재
	GitHubDefault      func() (string, bool)  // gh 로 조회한 GitHub default
	SymbolicRef        func() (string, bool)  // origin/HEAD 가 가리키는 이름
}

// Default 는 main → master → GitHub → origin/HEAD 순으로 default branch 를 정한다.
// 어느 단계로도 정하지 못하면 ok=false 를 돌려준다.
func Default(d Deps) (string, bool) {
	if d.RemoteBranchExists("main") {
		return "main", true
	}
	if d.RemoteBranchExists("master") {
		return "master", true
	}
	if name, ok := d.GitHubDefault(); ok && name != "" {
		return name, true
	}
	if name, ok := d.SymbolicRef(); ok && name != "" {
		return name, true
	}
	return "", false
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/git-update-default && go test ./internal/resolve/... -v`
Expected: 5개 테스트 모두 PASS.

- [ ] **Step 5: 커밋**

```bash
git add apps/git-update-default/internal/resolve
git commit -m "feat(git-update-default): default branch 우선순위 탐색"
```

---

## Task 4: confirm 패키지 — dirty 3지선다 모델과 TUI

dirty 일 때 보여줄 3지선다(stash/force/취소)의 Action 타입, TTY 감지, bubbletea 단일 선택 TUI 를 만든다. 상태 전이는 순수 함수로 테스트한다.

**Files:**
- Create: `apps/git-update-default/internal/confirm/confirm.go`
- Create: `apps/git-update-default/internal/confirm/tui.go`
- Test: `apps/git-update-default/internal/confirm/confirm_test.go`

- [ ] **Step 1: confirm.go 작성(Action·TTY 감지)**

`apps/git-update-default/internal/confirm/confirm.go`:

```go
// Package confirm 은 dirty 작업 트리에 대한 3지선다(stash/force/취소)를 받는다.
package confirm

import (
	"os"

	"golang.org/x/term"
)

// Action 은 dirty 일 때 사용자가 고른 처리다.
type Action int

const (
	// ActionCancel 은 아무것도 바꾸지 않고 멈춘다. 기본값(zero value)이다.
	ActionCancel Action = iota
	// ActionStash 는 변경을 stash 한 뒤 진행한다.
	ActionStash
	// ActionForce 는 추적 변경을 버리고 진행한다.
	ActionForce
)

// IsTTY 는 표준 입력이 터미널인지 본다. 아니면 인터랙티브 선택을 띄울 수 없다.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
```

- [ ] **Step 2: TUI 상태 전이 실패 테스트 작성**

`apps/git-update-default/internal/confirm/confirm_test.go`:

```go
package confirm

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestModelDefaultsToCancel(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	// 초기 커서가 취소이므로 enter 를 바로 누르면 취소가 선택된다.
	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := got.(model)
	if !fm.done || fm.chosen != ActionCancel {
		t.Fatalf("default enter -> chosen=%v done=%v, want ActionCancel,true", fm.chosen, fm.done)
	}
}

func TestModelStashShortcut(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	got, _ := m.Update(keyRune('s'))
	fm := got.(model)
	if !fm.done || fm.chosen != ActionStash {
		t.Fatalf("'s' -> chosen=%v done=%v, want ActionStash,true", fm.chosen, fm.done)
	}
}

func TestModelForceShortcut(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	got, _ := m.Update(keyRune('f'))
	fm := got.(model)
	if !fm.done || fm.chosen != ActionForce {
		t.Fatalf("'f' -> chosen=%v done=%v, want ActionForce,true", fm.chosen, fm.done)
	}
}

func TestModelArrowToStashThenEnter(t *testing.T) {
	// 커서 순서: stash(0) / force(1) / 취소(2). 초기 커서는 취소(2).
	m := newModel([]string{" M f.txt"})
	up1, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})         // 2 -> 1 (force)
	up2, _ := up1.(model).Update(tea.KeyMsg{Type: tea.KeyUp}) // 1 -> 0 (stash)
	got, _ := up2.(model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := got.(model)
	if fm.chosen != ActionStash {
		t.Fatalf("up up enter -> chosen=%v, want ActionStash", fm.chosen)
	}
}

func TestModelEscCancels(t *testing.T) {
	m := newModel([]string{" M f.txt"})
	// 커서를 force 로 옮겨도 esc 는 취소다.
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got, _ := up.(model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	fm := got.(model)
	if !fm.done || fm.chosen != ActionCancel {
		t.Fatalf("esc -> chosen=%v done=%v, want ActionCancel,true", fm.chosen, fm.done)
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `cd apps/git-update-default && go test ./internal/confirm/... 2>&1 | head`
Expected: 컴파일 실패(`undefined: newModel`, `undefined: model`).

- [ ] **Step 4: tui.go 구현**

`apps/git-update-default/internal/confirm/tui.go`:

```go
package confirm

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// choice 는 선택 한 줄이다.
type choice struct {
	action Action
	key    rune   // 단축키
	label  string // 표시 문구
}

// choices 의 순서가 화면 표시 순서이자 커서 인덱스다. 표시 순서는 stash → force →
// 취소이며, 초기 커서는 마지막(취소)에 둔다. 되돌릴 수 없는 force 의 오선택을
// 줄이기 위함이다 — 이 순서와 초기 커서를 바꾸면 안전 기본값이 깨진다.
var choices = []choice{
	{ActionStash, 's', "stash 후 진행 — 변경을 보관하고 default branch 로 전환"},
	{ActionForce, 'f', "force — 추적 변경을 버리고 진행 (되돌릴 수 없음)"},
	{ActionCancel, 'c', "취소 — 아무것도 바꾸지 않고 멈춤"},
}

type model struct {
	files  []string
	cursor int
	chosen Action
	done   bool
}

func newModel(files []string) model {
	return model{files: files, cursor: len(choices) - 1, chosen: ActionCancel}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case k.Type == tea.KeyCtrlC || k.Type == tea.KeyEsc:
			m.chosen, m.done = ActionCancel, true
			return m, tea.Quit
		case k.Type == tea.KeyEnter:
			m.chosen, m.done = choices[m.cursor].action, true
			return m, tea.Quit
		case k.Type == tea.KeyUp:
			m.move(-1)
		case k.Type == tea.KeyDown:
			m.move(1)
		case k.Type == tea.KeyRunes && len(k.Runes) == 1:
			switch k.Runes[0] {
			case 'k':
				m.move(-1)
			case 'j':
				m.move(1)
			default:
				for _, c := range choices {
					if c.key == k.Runes[0] {
						m.chosen, m.done = c.action, true
						return m, tea.Quit
					}
				}
			}
		}
	}
	return m, nil
}

func (m *model) move(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(choices)-1 {
		m.cursor = len(choices) - 1
	}
}

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleHelp  = lipgloss.NewStyle().Faint(true)
	styleDim   = lipgloss.NewStyle().Faint(true)
	styleCur   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
)

func (m model) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(
		fmt.Sprintf("커밋되지 않은 변경 %d개 — 어떻게 할까요?", len(m.files))) + "\n\n")
	for _, f := range m.files {
		b.WriteString(styleDim.Render("  "+f) + "\n")
	}
	b.WriteString("\n")
	for i, c := range choices {
		line := fmt.Sprintf("[%c] %s", c.key, c.label)
		if i == m.cursor {
			b.WriteString(styleCur.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + styleHelp.Render("↑↓/jk 이동 · enter 선택 · s/f/c 바로가기 · esc 취소"))
	return b.String()
}

// Run 은 bubbletea 단일 선택 화면을 띄워 Action 을 돌려준다. TTY 가 아니어서
// 프로그램 시작에 실패하면 ActionCancel 을 돌려준다(호출자는 IsTTY 로 미리 거른다).
func Run(files []string) Action {
	final, err := tea.NewProgram(newModel(files)).Run()
	if err != nil {
		return ActionCancel
	}
	fm, _ := final.(model)
	return fm.chosen
}
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `cd apps/git-update-default && go test ./internal/confirm/... -v`
Expected: 5개 테스트 모두 PASS.

- [ ] **Step 6: 커밋**

```bash
git add apps/git-update-default/internal/confirm
git commit -m "feat(git-update-default): dirty 3지선다 모델과 TUI"
```

---

## Task 5: 오케스트레이션 — run() 본체와 dirtyPath 분기

지금까지의 패키지를 main.go 에서 조립한다. dirty 처리 경로 결정(`dirtyPath`)을 순수 함수로 분리해 테스트하고, `run()` 에서 전체 흐름을 잇는다.

**Files:**
- Modify: `apps/git-update-default/cmd/git-update-default/main.go`(Task 1 의 `run` 스텁 교체, `dirtyPath` 등 추가)
- Modify: `apps/git-update-default/cmd/git-update-default/main_test.go`(parseArgs·dirtyPath 테스트 추가)

- [ ] **Step 1: parseArgs·dirtyPath 실패 테스트 추가**

`apps/git-update-default/cmd/git-update-default/main_test.go` 에 아래를 덧붙인다:

```go
func TestParseArgs(t *testing.T) {
	cases := []struct {
		args      []string
		wantStash bool
		wantForce bool
		wantErr   bool
	}{
		{nil, false, false, false},
		{[]string{"--stash"}, true, false, false},
		{[]string{"--force"}, false, true, false},
		{[]string{"--bogus"}, false, false, true},
	}
	for _, c := range cases {
		o, err := parseArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Fatalf("parseArgs(%v) err=%v wantErr=%v", c.args, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if o.stash != c.wantStash || o.force != c.wantForce {
			t.Fatalf("parseArgs(%v) = %+v", c.args, o)
		}
	}
}

func TestDirtyPath(t *testing.T) {
	cases := []struct {
		tty, stash, force bool
		want              dirtyAction
	}{
		{true, false, false, pathInteractive},
		{false, false, false, pathCancel},
		{false, true, false, pathStash},
		{false, false, true, pathForce},
		{true, true, false, pathStash}, // 플래그가 있으면 TTY 여도 묻지 않는다
		{true, false, true, pathForce},
		{true, true, true, pathForce}, // force 가 stash 보다 우선
	}
	for _, c := range cases {
		got := dirtyPath(c.tty, c.stash, c.force)
		if got != c.want {
			t.Fatalf("dirtyPath(tty=%v,stash=%v,force=%v) = %v want %v",
				c.tty, c.stash, c.force, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-update-default && go test ./cmd/... 2>&1 | head`
Expected: 컴파일 실패(`undefined: dirtyAction`, `undefined: dirtyPath`, `pathInteractive` 등).

- [ ] **Step 3: main.go 의 run 스텁을 본구현으로 교체**

Task 1 에서 둔 임시 스텁 `func run(opts options) int { ... 아직 구현되지 않았습니다 ... }` 를 통째로 아래로 교체한다. `parseArgs`·`options`·`versionLine`·`main`·`helpText` 는 그대로 둔다. import 블록을 다음으로 바꾼다:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silee-tools/git-update-default/internal/confirm"
	"github.com/silee-tools/git-update-default/internal/gitx"
	"github.com/silee-tools/git-update-default/internal/resolve"
)
```

`run` 스텁을 다음으로 교체한다:

```go
// dirtyAction 은 dirty 작업 트리를 만났을 때 따라갈 경로다.
type dirtyAction int

const (
	pathInteractive dirtyAction = iota // TUI 로 묻는다
	pathStash                          // 묻지 않고 stash
	pathForce                          // 묻지 않고 추적 변경 폐기
	pathCancel                         // 묻지 않고 멈춘다
)

// dirtyPath 는 환경(TTY 여부)과 플래그로 dirty 처리 경로를 정한다.
// 플래그(--force/--stash)가 있으면 TTY 여도 묻지 않는다. force 가 stash 보다 우선한다.
// 플래그가 없으면 TTY 일 때만 인터랙티브로 묻고, 비-TTY 면 취소로 안전하게 멈춘다.
func dirtyPath(tty, stash, force bool) dirtyAction {
	switch {
	case force:
		return pathForce
	case stash:
		return pathStash
	case tty:
		return pathInteractive
	default:
		return pathCancel
	}
}

// run 은 git-update-default 본체다. 종료 코드를 돌려준다.
func run(opts options) int {
	if !gitx.IsRepo() {
		fmt.Fprintln(os.Stderr, "git-update-default: git 저장소가 아닙니다.")
		return 1
	}
	if !gitx.HasOriginRemote() {
		fmt.Fprintln(os.Stderr, "git-update-default: origin 원격이 없어 default branch 를 정할 수 없습니다.")
		return 1
	}
	if err := gitx.FetchPrune(); err != nil {
		// fetch 실패(오프라인 등)는 치명적이지 않다. 로컬에 이미 있는 원격 추적
		// 참조로 진행하되, 최신이 아닐 수 있음을 알린다.
		fmt.Fprintln(os.Stderr, "경고: git fetch 실패 — 로컬의 원격 추적 정보로 진행합니다.")
	}

	branch, ok := resolve.Default(resolve.Deps{
		RemoteBranchExists: gitx.RemoteBranchExists,
		GitHubDefault:      gitx.GitHubDefault,
		SymbolicRef:        gitx.SymbolicRefDefault,
	})
	if !ok {
		fmt.Fprintln(os.Stderr, "git-update-default: default branch 를 정할 수 없습니다 (main·master·gh·origin/HEAD 모두 실패).")
		return 1
	}

	files, err := gitx.DirtyFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-update-default:", err)
		return 1
	}
	if len(files) > 0 {
		if code := handleDirty(files, opts); code != 0 {
			return code
		}
	}

	if code := switchTo(branch); code != 0 {
		return code
	}

	if err := gitx.MergeFFOnly(branch); err != nil {
		fmt.Fprintf(os.Stderr, "git-update-default: %s 가 origin/%s 와 갈라져 fast-forward 할 수 없습니다.\n", branch, branch)
		fmt.Fprintln(os.Stderr, "  → 직접 rebase·reset 으로 정리하세요. 강제로 맞추지 않습니다.")
		return 1
	}

	fmt.Printf("✓ %s 로 전환하고 origin/%s 최신까지 맞췄습니다.\n", branch, branch)
	return 0
}

// handleDirty 는 dirty 작업 트리를 처리한다. 인터랙티브 취소는 정상 흐름이므로
// 그 자리에서 os.Exit(0) 으로 끝낸다. stash·force 처리 중 실패하면 0 이 아닌 코드를,
// 비-TTY 에서 처리 수단이 없어 멈추면 1 을 돌려준다.
func handleDirty(files []string, opts options) int {
	action := confirm.ActionCancel
	switch dirtyPath(confirm.IsTTY(), opts.stash, opts.force) {
	case pathInteractive:
		action = confirm.Run(files)
	case pathStash:
		action = confirm.ActionStash
	case pathForce:
		action = confirm.ActionForce
	case pathCancel:
		printDirty(files)
		fmt.Fprintln(os.Stderr, "git-update-default: 커밋되지 않은 변경이 있습니다. --stash 또는 --force 를 쓰거나 직접 정리하세요.")
		return 1
	}

	switch action {
	case confirm.ActionCancel:
		fmt.Println("취소했습니다. 아무것도 바꾸지 않았습니다.")
		os.Exit(0)
	case confirm.ActionStash:
		if err := gitx.StashPush(); err != nil {
			fmt.Fprintln(os.Stderr, "git-update-default: stash 실패:", err)
			return 1
		}
		cur := gitx.CurrentBranch()
		fmt.Printf("변경을 stash 했습니다. 원래 브랜치(%s)로 돌아가 `git stash pop` 으로 복원하세요.\n", cur)
	case confirm.ActionForce:
		if err := gitx.ResetHard(); err != nil {
			fmt.Fprintln(os.Stderr, "git-update-default: reset 실패:", err)
			return 1
		}
		fmt.Println("추적 변경을 버렸습니다.")
	}
	return 0
}

// printDirty 는 변경 파일 목록을 그대로 출력한다(비-TTY·취소 안내용).
func printDirty(files []string) {
	fmt.Fprintf(os.Stderr, "커밋되지 않은 변경 %d개:\n", len(files))
	for _, f := range files {
		fmt.Fprintln(os.Stderr, "  "+f)
	}
}

// switchTo 는 default branch 로 전환한다. 이미 그 브랜치면 아무것도 하지 않는다.
func switchTo(branch string) int {
	if gitx.CurrentBranch() == branch {
		return 0
	}
	var err error
	if gitx.LocalBranchExists(branch) {
		err = gitx.Switch(branch)
	} else {
		err = gitx.SwitchCreateTracking(branch)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-update-default: 브랜치 전환 실패:", err)
		return 1
	}
	return 0
}
```

불변조건: `handleDirty` 의 두 종료를 섞지 말 것 — 인터랙티브 `ActionCancel` 은 사용자가 정상적으로 무르는 것이라 `os.Exit(0)`, `pathCancel`(비-TTY·플래그 없음)은 처리 수단이 없어 멈춘 것이라 `return 1` 이다.

- [ ] **Step 4: 전체 테스트·빌드 확인**

Run: `cd apps/git-update-default && go test ./... && go build ./cmd/git-update-default`
Expected: 모든 패키지 테스트 PASS, 빌드 성공.

- [ ] **Step 5: 타입 체크·vet(별도 게이트)**

Run: `cd apps/git-update-default && go vet ./...`
Expected: 출력 없음(통과).

- [ ] **Step 6: 커밋**

```bash
git add apps/git-update-default/cmd
git commit -m "feat(git-update-default): run 오케스트레이션과 dirty 경로 분기"
```

---

## Task 6: 자동완성·README·CI·릴리스 등록

도구를 저장소의 새 도구 체크리스트대로 배선한다.

**Files:**
- Create: `apps/git-update-default/completions/_git-update-default`
- Create: `apps/git-update-default/completions/git-update-default.bash`
- Create: `apps/git-update-default/README.md`
- Create: `.github/workflows/git-update-default-ci.yml`
- Modify: `release-please-config.json`
- Modify: `.release-please-manifest.json`
- Modify: `README.md`(루트)
- Modify: `docs/README_ko.md`

- [ ] **Step 1: 자동완성 작성**

`apps/git-update-default/completions/_git-update-default`:

```
#compdef git-update-default

_arguments \
  '--stash[dirty 일 때 묻지 않고 stash 후 진행]' \
  '--force[dirty 일 때 묻지 않고 추적 변경을 버리고 진행]' \
  '--version[버전 출력]' \
  '--help[도움말 출력]'
```

`apps/git-update-default/completions/git-update-default.bash`:

```bash
_git_update_default() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local opts="--stash --force --version --help"
  COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
}
complete -o nosort -F _git_update_default git-update-default
```

- [ ] **Step 2: README 작성**

`apps/git-update-default/README.md` 를 만든다. 본문은 도구 목적·동작 5단계·사용 예시·설치를 담는다. 코드 예시 블록은 일반 펜스(```)로 쓴다. 내용:

```markdown
# git-update-default

현재 위치가 속한 git 저장소를 원격 default branch 의 최신 상태로 전환하는 명령줄 도구.
저장소 안 어느 하위 경로에서 실행해도 동작한다.

## 동작

1. 현재 위치가 git 저장소인지, origin 원격이 있는지 확인한다.
2. git fetch origin --prune 으로 원격 최신을 받는다.
3. default branch 를 main → master → gh(GitHub default) → origin/HEAD 순으로 정한다.
4. 커밋되지 않은 변경이 있으면 파일 목록을 보여주고 stash / force / 취소(기본값) 를 묻는다.
5. default branch 로 전환하고 origin/<default> 최신까지 fast-forward 한다.
   갈라져 fast-forward 가 불가능하면 경고만 하고 멈춘다(강제하지 않음).

## 사용

    git update-default          # 또는 git-update-default
    git update-default --stash  # dirty 일 때 묻지 않고 stash 후 진행
    git update-default --force  # dirty 일 때 묻지 않고 추적 변경 폐기 후 진행

## 설치

Homebrew tap(silee-tools/homebrew-tap)으로 설치하거나, 개발 빌드는
mise run install 로 ~/.local/bin 에 둔다.
```

- [ ] **Step 3: CI 워크플로우 작성**

`.github/workflows/git-update-default-ci.yml`:

```yaml
name: git-update-default CI

on:
  push:
    paths:
      - 'apps/git-update-default/**'
      - '.github/workflows/git-update-default-ci.yml'
  pull_request:
    paths:
      - 'apps/git-update-default/**'
      - '.github/workflows/git-update-default-ci.yml'

jobs:
  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: apps/git-update-default
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v4
        with:
          working_directory: apps/git-update-default
      - run: mise run fmt-check
      - run: mise run lint
      - run: mise run test
      - run: mise run build
```

- [ ] **Step 4: release-please 등록**

`release-please-config.json` 의 `packages` 블록에 한 줄을 더해 다음과 같이 만든다:

```json
  "packages": {
    "apps/jg": {"package-name": "jg"},
    "apps/totp": {"package-name": "totp"},
    "apps/git-tidy": {"package-name": "git-tidy"},
    "apps/git-update-default": {"package-name": "git-update-default"}
  }
```

`.release-please-manifest.json` 을 다음과 같이 만든다:

```json
{
  "apps/jg": "0.6.0",
  "apps/totp": "1.1.1",
  "apps/git-tidy": "0.7.1",
  "apps/git-update-default": "0.0.0"
}
```

(jg·totp·git-tidy 의 버전 값은 현재 파일의 값을 그대로 두고 새 줄만 추가한다. 위 값은 작성 시점 기준이며, 실제 파일의 현재 값과 다르면 현재 값을 보존한다.)

- [ ] **Step 5: 루트·한글 README 도구 표 갱신**

먼저 두 README 의 git-tidy 행 위치·열 형식을 확인한다:

Run: `grep -n "git-tidy" README.md docs/README_ko.md`

확인한 열 형식과 동일하게 git-update-default 행을 추가한다. 설명 문구는 "현재 저장소를 원격 default branch 최신으로 전환"으로 통일한다.

- [ ] **Step 6: 형식·버전 게이트 확인**

Run: `cd apps/git-update-default && mise run fmt-check`
Expected: 통과.

버전 형식 게이트 사용법을 먼저 확인한 뒤 실행한다:

Run: `head -30 scripts/check-version-format.sh`
그 사용법대로 게이트를 돌려 git-update-default 가 포함돼 통과하는지 본다(이 게이트는 `apps/*` 를 자동 순회하며 `.goreleaser.yaml` 의 goos 로 OS 를 라우팅한다. 우리 도구는 darwin+linux 이므로 두 경로 모두 대상).

- [ ] **Step 7: 커밋**

```bash
git add apps/git-update-default/completions apps/git-update-default/README.md .github/workflows/git-update-default-ci.yml release-please-config.json .release-please-manifest.json README.md docs/README_ko.md
git commit -m "feat(git-update-default): 자동완성·CI·릴리스 등록과 문서"
```

---

## Task 7: E2E 검증 (1회성 수동, expect)

mock 단위 테스트가 못 잡는 실제 git 경로(전환·최신화·dirty 선택)를 임시 저장소에서 종단 간으로 한 번 검증한다. TUI 는 TTY 가 필요하므로 expect 로 PTY 를 할당한다. 먼저 `cd apps/git-update-default && mise run install` 로 `~/.local/bin/git-update-default` 를 빌드해 둔다.

**Files:** (테스트용 임시 디렉터리, 저장소에 커밋하지 않음)

- [ ] **Step 1: 세션 전용 작업 디렉터리에 origin + 클론 구성**

```bash
WORK="${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}/gud-e2e"
mkdir -p "$WORK" && cd "$WORK"
git init -b main --bare origin.git
git clone origin.git repo && cd repo
git config user.email t@e.com && git config user.name t
echo hi > a.txt && git add . && git commit -m init && git push -u origin main
git switch -c feature && echo more >> a.txt && git commit -am more && git push -u origin feature
# origin 의 main 을 한 커밋 앞서게 만든 뒤 feature 로 돌아온다
git switch main && echo base >> a.txt && git commit -am base && git push && git switch feature
```

- [ ] **Step 2: clean + 하위 경로 실행 (전환·최신화 + 재귀 동작 확인)**

```bash
cd "$WORK/repo" && mkdir -p sub && cd sub
~/.local/bin/git-update-default
cd "$WORK/repo"
git branch --show-current   # main 이어야 함
git log --oneline -1        # origin/main 최신(base 커밋)이어야 함
```

Expected: `main` 으로 전환됨, 최신 `base` 커밋 반영. 하위 경로(sub/)에서 실행해도 저장소 전체가 대상이 된다.

- [ ] **Step 3: dirty + 취소 (expect, 기본값 보존 확인)**

```bash
cd "$WORK/repo" && git switch feature && echo dirty >> a.txt
expect -c '
  spawn ~/.local/bin/git-update-default
  expect "어떻게 할까요"
  send "\r"
  expect eof
'
git branch --show-current   # feature 그대로여야 함 (취소)
git status --porcelain      # dirty 그대로여야 함
```

Expected: feature 브랜치 유지, 변경 보존(취소 동작).

- [ ] **Step 4: dirty + --stash (비대화 우회 확인)**

```bash
cd "$WORK/repo" && git switch feature
git stash list   # 비어 있어야 함
~/.local/bin/git-update-default --stash
git branch --show-current   # main 이어야 함
git stash list              # stash 1개 있어야 함
```

Expected: main 으로 전환, stash 1개 생성, 복원 안내 출력.

- [ ] **Step 5: 증거 기록 + 정리**

위 세 실행의 출력(전환된 브랜치, 최신 커밋, stash 목록, 취소 시 보존)을 PR 본문 E2E 증거 섹션에 붙인다. 그 뒤 임시 디렉터리를 통째로 지운다:

```bash
rm -rf "${CLAUDE_CODE_TMPDIR:-${TMPDIR:-/tmp}}/session-${CLAUDE_CODE_SESSION_ID:-$$}/gud-e2e"
```

---

## Task 8: 기존 git-sync-main 정리 (별도 단계 — 사용자 머신 설정)

이 Task 는 저장소 코드가 아니라 사용자 머신 설정을 바꾼다. 도구 구현 PR 과 분리해 다루며, 실행 전에 사용자에게 한 번 더 확인한다(chezmoi 소스 변경 + git config 변경은 되돌리기 번거로운 작업이다).

- [ ] **Step 1: 정리 대상 현황 확인**

```bash
ls -la ~/.local/bin/git-sync-main
git config --global --get alias.sync-main
grep -n "git sync-main\|gsm=\|qq=" ~/.zshrc
chezmoi managed | grep -E 'zshrc|local/bin/git-sync-main'
chezmoi status
```

`chezmoi status` 가 dirty 면 임의로 진행하지 않고, drift 내용을 `chezmoi diff` 로 확인한 뒤 어떻게 정리할지 사용자에게 먼저 묻는다.

- [ ] **Step 2: git alias 제거**

```bash
git config --global --unset alias.sync-main
```

- [ ] **Step 3: 깨진 심볼릭 링크 제거**

`~/.local/bin/git-sync-main` 이 chezmoi managed 가 아니면 직접 제거하고, managed 면 chezmoi 소스에서 제거 후 apply 한다.

```bash
# managed 가 아닌 경우
rm ~/.local/bin/git-sync-main
```

- [ ] **Step 4: zshrc alias 교체 (chezmoi 소스 편집)**

`~/.zshrc` 는 chezmoi managed 이므로 홈 타깃이 아니라 소스를 편집한다.

```bash
chezmoi source-path ~/.zshrc   # 소스 파일 경로 확인
```

소스 파일에서 `alias gsm='git sync-main'` 줄을 `alias gud='git update-default'` 로, `alias qq='gsm && gtidy! && logout'` 을 `alias qq='gud && gtidy! && logout'` 으로 바꾼다. 그 뒤 diff 로 변경이 의도 범위뿐임을 확인하고 적용한다:

```bash
chezmoi diff ~/.zshrc
chezmoi apply ~/.zshrc
```

- [ ] **Step 5: 정리 검증**

```bash
git config --global --get alias.sync-main   # 출력 없어야 함
ls ~/.local/bin/git-sync-main 2>&1          # 없어야 함
zsh -ic 'type gud; type qq'                  # gud=git update-default, qq=gud && gtidy! && logout
```

---

## Self-Review

**1. Spec coverage** — spec 의 각 요구를 태스크에 대응:
- git root 하위 경로 실행 → gitx.IsRepo(git 자동 탐색), Task 7 Step 2 에서 sub/ 실행 검증.
- default branch 동적 탐색(main→master→gh→origin/HEAD) → Task 3 resolve + Task 2 gitx.
- fetch 최신화 → run() 의 FetchPrune + MergeFFOnly.
- dirty 3지선다(stash/force/취소, 기본 취소) → Task 4 confirm + Task 5 handleDirty.
- stash 자동 복원 안 함 + 안내 → handleDirty ActionStash 경로.
- force = 추적 변경만 폐기(untracked 보존) → gitx.ResetHard(reset --hard HEAD).
- diverge 시 강제 안 하고 경고 → run() MergeFFOnly 실패 분기.
- 비-TTY dirty 중단 + --stash/--force 우회 → dirtyPath + handleDirty.
- 버전 형식 → Task 1 versionLine + Task 6 게이트 확인.
- bubbletea v1.3.4 핀 → Task 1 go.mod.
- 저장소 편입 산출물 → Task 6.
- 기존 git-sync-main 정리 → Task 8.

**2. Placeholder scan** — 모든 코드 step 에 완전한 코드가 있다. Task 6 Step 5(루트 README)와 Step 6(버전 게이트)은 저장소마다 형식·사용법이 달라 grep·head 로 현재 형식을 확인한 뒤 그에 맞추도록 명시했고, 추가할 내용(행 형식·통과 기준)은 구체적으로 적었다.

**3. Type consistency** — `options{stash,force}` 는 Task 1·5 일치. `dirtyAction`/`pathInteractive|pathStash|pathForce|pathCancel` 는 Task 5 안에서 정의·사용 일치. `confirm.Action`/`ActionCancel|ActionStash|ActionForce` 는 Task 4 정의·Task 5 사용 일치. `resolve.Deps{RemoteBranchExists,GitHubDefault,SymbolicRef}` 는 Task 3 정의, Task 5 에서 `gitx.RemoteBranchExists`(func(string)bool)·`gitx.GitHubDefault`(func()(string,bool))·`gitx.SymbolicRefDefault`(func()(string,bool))로 주입 — 시그니처 일치.
