# git-tidy multi-call (gtidy / gtidy!) 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git-tidy 바이너리가 `gtidy`(=`git-tidy`)·`gtidy!`(=`git-tidy --run`) 두 추가 이름으로 호출되도록 multi-call 기능을 더한다.

**Architecture:** `main()` 이 `filepath.Base(os.Args[0])` 로 호출된 이름을 확인하고, 이름이 `gtidy!` 면 인자 앞에 `--run` 을 끼워 넣는다(`effectiveArgs` 순수 함수). `gtidy`·`gtidy!` 는 `git-tidy` 바이너리를 가리키는 심볼릭 링크이며, `.mise.toml` 의 install 태스크와 Homebrew formula 가 설치 시점에 만든다.

**Tech Stack:** Go 1.23, mise, GoReleaser, Homebrew.

설계 근거는 [docs/plans/2026-05-22-git-tidy-multicall-design.md](2026-05-22-git-tidy-multicall-design.md) 를 단일 기준으로 삼는다. 완료 후 전체 검토는 이 계획서가 아니라 그 설계 문서에 대조한다.

---

## File Structure

```
apps/git-tidy/
  cmd/git-tidy/main.go         effectiveArgs + argv[0] 분기 + helpText 갱신
  cmd/git-tidy/main_test.go    effectiveArgs 단위 테스트
  .mise.toml                   install/uninstall 태스크에 심볼릭 링크
  completions/_git-tidy        zsh 자동완성 — 세 이름 커버
  completions/git-tidy.bash    bash 자동완성 — 세 이름 커버
docs/plans/2026-05-22-git-tidy-cleanup-model.md   셸 별칭 관련 문장 갱신
apps/git-tidy/PRD.md           기능 범위에 gtidy/gtidy! 추가
apps/git-tidy/README.md        사용법에 gtidy/gtidy! 추가
```

별도 저장소 `homebrew-tap` 의 `Formula/git-tidy.rb` 도 갱신한다(Task 5).

---

## Task 1: argv[0] 분기 + effectiveArgs (main.go)

**Files:**
- Modify: `apps/git-tidy/cmd/git-tidy/main.go`, `apps/git-tidy/cmd/git-tidy/main_test.go`

`go` 는 mise 로 관리된다. 명령은 `cd apps/git-tidy && mise run test` 처럼 실행한다.

- [ ] **Step 1: 실패하는 테스트 작성**

`apps/git-tidy/cmd/git-tidy/main_test.go` 의 import 를 다음으로 바꾼다(현재는 `import "testing"` 한 줄):
```go
import (
	"reflect"
	"testing"
)
```

그리고 파일 끝에 다음 테스트를 추가한다:
```go
func TestEffectiveArgs(t *testing.T) {
	cases := []struct {
		invoked string
		args    []string
		want    []string
	}{
		{"git-tidy", []string{"--no-fetch"}, []string{"--no-fetch"}},
		{"gtidy", []string{"--stale-days=7"}, []string{"--stale-days=7"}},
		{"gtidy!", nil, []string{"--run"}},
		{"gtidy!", []string{"--no-tui"}, []string{"--run", "--no-tui"}},
	}
	for _, c := range cases {
		got := effectiveArgs(c.invoked, c.args)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("effectiveArgs(%q, %v) = %v, want %v", c.invoked, c.args, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/git-tidy && go test ./cmd/git-tidy/`
Expected: FAIL — `effectiveArgs` 미정의로 빌드 실패.

- [ ] **Step 3: main.go 구현**

`apps/git-tidy/cmd/git-tidy/main.go` 에 네 가지를 적용한다.

(3-1) import 블록에 `"path/filepath"` 를 추가한다. 최종 import 블록:
```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/silee-tools/git-tidy/internal/classify"
	"github.com/silee-tools/git-tidy/internal/gitx"
	"github.com/silee-tools/git-tidy/internal/pick"
)
```

(3-2) `versionLine` 함수 바로 다음에 `effectiveArgs` 를 추가한다:
```go
// effectiveArgs 는 호출된 이름에 따라 실제로 쓸 인자 목록을 돌려준다.
// gtidy! 로 불리면 인자 앞에 --run 을 끼워 넣고, 그 외에는 인자를 그대로 둔다.
func effectiveArgs(invoked string, args []string) []string {
	if invoked == "gtidy!" {
		return append([]string{"--run"}, args...)
	}
	return args
}
```

(3-3) `helpText` 상수 전체를 다음으로 바꾼다(끝에 "단축 명령" 절 추가):
```go
const helpText = `Usage: git-tidy [--run] [options]

작업이 끝났거나 오래 방치된 로컬 git 브랜치를 정리한다.

  git-tidy              dry-run — 삭제 대상만 표시
  git-tidy --run        삭제 대상을 다중 선택해 삭제
  git-tidy --run --no-tui  체크박스 TUI 대신 줄 기반 선택
  --stale-days=N        stale 판정 창 (기본 20, GIT_TIDY_STALE_DAYS)
  --no-fetch            git fetch --prune 건너뛰기
  -v, --version         버전 출력
  -h, --help            도움말 출력

단축 명령:
  gtidy                 git-tidy 와 동일
  gtidy!                git-tidy --run 과 동일
`
```

(3-4) `main()` 함수 전체를 다음으로 바꾼다:
```go
func main() {
	invoked := filepath.Base(os.Args[0])
	args := effectiveArgs(invoked, os.Args[1:])

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
		fmt.Fprintln(os.Stderr, "git-tidy:", err)
		fmt.Fprintln(os.Stderr, "git-tidy --help 로 사용법을 확인하세요.")
		os.Exit(1)
	}
	os.Exit(run(opts))
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/git-tidy && go test -count=1 ./...`
Expected: PASS — 기존 테스트 + `TestEffectiveArgs` 전원 통과.

- [ ] **Step 5: lint·fmt·build 확인**

Run: `cd apps/git-tidy && mise run fmt-check && mise run lint && mise run build`
Expected: 전부 통과. `effectiveArgs` 는 `main()` 이 호출하므로 `unused` 경고가 나지 않는다.

- [ ] **Step 6: Commit**

```bash
git add apps/git-tidy/cmd/git-tidy/
git commit -m "feat(git-tidy): dispatch gtidy/gtidy! via argv[0]"
```

---

## Task 2: 심볼릭 링크 설치 (.mise.toml)

**Files:**
- Modify: `apps/git-tidy/.mise.toml`

`jg` 의 install 태스크가 `jgw` 심볼릭 링크를 만드는 방식을 그대로 따른다.

- [ ] **Step 1: install·uninstall 태스크 교체**

`apps/git-tidy/.mise.toml` 의 `[tasks.install]` 과 `[tasks.uninstall]` 두 블록을 다음으로 바꾼다:
```toml
[tasks.install]
description = "Build and install to ~/.local/bin (git-tidy + gtidy/gtidy! symlinks)"
run = """
go build -o ~/.local/bin/git-tidy ./cmd/git-tidy
ln -sf ~/.local/bin/git-tidy ~/.local/bin/gtidy
ln -sf ~/.local/bin/git-tidy "$HOME/.local/bin/gtidy!"
"""

[tasks.uninstall]
description = "Remove local dev build"
run = """
rm -f ~/.local/bin/git-tidy ~/.local/bin/gtidy "$HOME/.local/bin/gtidy!"
"""
```
`gtidy!` 경로는 `!` 때문에 따옴표로 감싸야 하며, 따옴표 안에서는 `~` 가 확장되지 않으므로 `$HOME` 을 쓴다.

- [ ] **Step 2: 설치·검증·정리**

Run:
```bash
cd apps/git-tidy && mise run install
ls -l ~/.local/bin/git-tidy ~/.local/bin/gtidy "$HOME/.local/bin/gtidy!"
~/.local/bin/gtidy -v
"$HOME/.local/bin/gtidy!" -v
mise run uninstall
ls ~/.local/bin/git-tidy ~/.local/bin/gtidy "$HOME/.local/bin/gtidy!" 2>&1
```
Expected: `gtidy`·`gtidy!` 가 `git-tidy` 를 가리키는 심볼릭 링크로 만들어진다. `gtidy -v` 는 `gtidy v<버전> © 2026 silee-tools`, `gtidy! -v` 는 `gtidy! v<버전> © 2026 silee-tools` 를 출력한다. `mise run uninstall` 후 세 파일이 모두 없다.

- [ ] **Step 3: Commit**

```bash
git add apps/git-tidy/.mise.toml
git commit -m "feat(git-tidy): symlink gtidy/gtidy! in mise install task"
```

---

## Task 3: 자동완성 — 세 이름 커버 (completions)

**Files:**
- Modify: `apps/git-tidy/completions/_git-tidy`, `apps/git-tidy/completions/git-tidy.bash`

세 이름은 플래그 집합이 같으므로 완성 후보도 같다. 완성 파일 하나가 세 이름을 모두 등록한다.

- [ ] **Step 1: zsh 완성 갱신**

`apps/git-tidy/completions/_git-tidy` 의 첫 줄을 다음으로 바꾼다:
```
#compdef git-tidy gtidy gtidy!
```
나머지 줄(`_arguments` 블록)은 그대로 둔다.

- [ ] **Step 2: bash 완성 갱신**

`apps/git-tidy/completions/git-tidy.bash` 의 마지막 줄을 다음으로 바꾼다:
```bash
complete -o nosort -F _git_tidy git-tidy gtidy 'gtidy!'
```

- [ ] **Step 3: 구문 검사**

Run: `bash -n apps/git-tidy/completions/git-tidy.bash && zsh -n apps/git-tidy/completions/_git-tidy && echo OK`
Expected: `OK` — 두 파일 모두 셸 구문 오류 없음.

- [ ] **Step 4: Commit**

```bash
git add apps/git-tidy/completions/
git commit -m "feat(git-tidy): cover gtidy/gtidy! in shell completions"
```

---

## Task 4: 문서 갱신

**Files:**
- Modify: `docs/plans/2026-05-22-git-tidy-cleanup-model.md`, `apps/git-tidy/PRD.md`, `apps/git-tidy/README.md`

- [ ] **Step 1: 설계 문서(cleanup-model) 갱신**

`docs/plans/2026-05-22-git-tidy-cleanup-model.md` 의 "명령과 흐름" 절에 있는 다음 두 줄을 찾는다:
```
- 셸 별칭(`gtidy` 등)은 바이너리가 스스로 정의할 수 없으므로 제공하지 않는다.
  사용자가 원하면 직접 설정한다.
```
이를 다음으로 바꾼다:
```
- `gtidy` 와 `gtidy!` 는 `git-tidy` 바이너리를 가리키는 multi-call 단축 명령이다.
  `gtidy` 는 `git-tidy` 와, `gtidy!` 는 `git-tidy --run` 과 같다.
```

- [ ] **Step 2: PRD 갱신**

`apps/git-tidy/PRD.md` 를 읽고, "기능 범위" 절의 명령 목록에서 `git-tidy --help` 항목 바로 다음에 두 줄을 추가한다:
```
- `gtidy`: `git-tidy` 와 동일하게 동작하는 단축 명령이다.
- `gtidy!`: `git-tidy --run` 과 동일하게 동작하는 단축 명령이다.
```
주변 항목의 문장 형식과 들여쓰기에 맞춘다. PRD 는 영구 명세 문서이므로 작업 경위는 적지 않는다.

- [ ] **Step 3: README 갱신**

`apps/git-tidy/README.md` 를 읽고, 사용법을 설명하는 절에 `gtidy` 와 `gtidy!` 를 추가한다. `gtidy` 는 `git-tidy` 와 동일하고 `gtidy!` 는 `git-tidy --run` 과 동일하다는 점, 그리고 Homebrew·mise 설치 시 두 이름이 함께 깔린다는 점을 한두 문장으로 적는다. 주변 서술 형식에 맞춘다.

- [ ] **Step 4: Commit**

```bash
git add docs/plans/2026-05-22-git-tidy-cleanup-model.md apps/git-tidy/PRD.md apps/git-tidy/README.md
git commit -m "docs(git-tidy): document gtidy/gtidy! shortcuts"
```

---

## Task 5: Homebrew formula — 심볼릭 링크 설치

**Files:**
- Modify (별도 저장소): `/Users/silee/repos/silee-tools/homebrew-tap/Formula/git-tidy.rb`

이 변경은 `cli` 저장소가 아니라 `homebrew-tap` 저장소에서 한다.

- [ ] **Step 1: homebrew-tap 최신화**

Run: `cd /Users/silee/repos/silee-tools/homebrew-tap && git pull`
Expected: fast-forward. 릴리스 자동화가 올린 `chore(git-tidy): bump to v0.3.0 ...` 커밋을 받아, formula 가 실제 version·sha256 을 갖춘 상태가 된다.

- [ ] **Step 2: def install 에 심볼릭 링크 추가**

`Formula/git-tidy.rb` 의 `def install` 블록을 다음으로 바꾼다:
```ruby
  def install
    bin.install "git-tidy"
    bin.install_symlink "git-tidy" => "gtidy"
    bin.install_symlink "git-tidy" => "gtidy!"
    zsh_completion.install "completions/_git-tidy"
    bash_completion.install "completions/git-tidy.bash" => "git-tidy"
  end
```
`version`·`sha256`·`url` 줄은 건드리지 않는다.

- [ ] **Step 3: 구문 검사**

Run: `cd /Users/silee/repos/silee-tools/homebrew-tap && ruby -c Formula/git-tidy.rb`
Expected: `Syntax OK`.

- [ ] **Step 4: Commit (homebrew-tap 저장소)**

`homebrew-tap` 저장소의 커밋 컨벤션(`git log --oneline -5` 로 확인)을 따라 커밋한다:
```bash
cd /Users/silee/repos/silee-tools/homebrew-tap
git add Formula/git-tidy.rb
git commit -m "feat(git-tidy): install gtidy/gtidy! symlinks"
```
push 하지 않는다.

---

## Task 6: 전체 검증 + 1회성 E2E

**Files:** 없음(검증만).

- [ ] **Step 1: 전체 테스트·lint·빌드**

Run: `cd apps/git-tidy && go test -count=1 ./... && mise run lint && mise run fmt-check && mise run build`
Expected: 전부 통과.

- [ ] **Step 2: 버전 conformance 게이트**

Run: 저장소 루트에서 `bash scripts/check-version-format.sh`
Expected: PASS. `git-tidy` 는 `git-tidy` 이름으로 호출되므로 `git-tidy -v` 가 `git-tidy v<버전> © 2026 silee-tools` 를 그대로 출력해 게이트를 통과한다.

- [ ] **Step 3: argv[0] 분기 1회성 E2E**

심볼릭 링크 분기와 `--run` 주입을 실제로 검증한다. 임시 디렉터리에서:
1. `git-tidy` 바이너리를 빌드하고, 같은 디렉터리에 `gtidy` 와 `gtidy!` 심볼릭 링크를 만든다.
2. `./gtidy -v` 와 `./gtidy! -v` 를 실행해 각각 `gtidy v<버전> ...`, `gtidy! v<버전> ...` 을 출력하는지 확인한다(호출 이름이 버전 줄에 반영됨).
3. `[gone]`·merged·stale 중 하나 이상으로 삭제 후보가 생기는 임시 git 저장소를 만든다.
4. 그 저장소에서 stdin 을 파이프로 연결한(터미널 아님) 채 `./gtidy` 를 실행한다 → 삭제 대상을 출력하고 아무것도 지우지 않으며 `--run` 안내를 출력한다.
5. 같은 저장소에서 stdin 을 파이프로 연결한 채 `./gtidy!` 를 실행한다 → 삭제 대상을 출력한 뒤 "삭제하려면 터미널이 필요합니다" 오류로 끝난다. 이것은 `gtidy!` 가 `--run` 을 주입해 삭제 경로(`runDeletion` → `DetectMode` → `ModeNone`)로 들어갔음을 증명한다. 터미널이 없어도 `gtidy` 와 `gtidy!` 의 동작 차이가 관찰된다.

각 단계의 셸 출력을 증거로 보관해 PR 본문 검증 절에 첨부한다.

---

## 자체 검토 메모

- 설계 문서의 모든 절(동작·왜 바이너리인가·multi-call 구현·심볼릭 링크 설치·자동완성·검증·문서 갱신)이 Task 1~6 에 대응된다.
- `effectiveArgs(invoked string, args []string) []string` 는 Task 1 에서 정의하고 Task 1·Task 6 에서 같은 시그니처로 쓰인다.
- `gtidy!` 의 `!` 가 zsh `#compdef` 에서 문제되면 `git-tidy`·`gtidy` 완성은 유지되고 `gtidy!` 만 완성에서 빠진다 — 설계 문서가 허용한 범위다.
