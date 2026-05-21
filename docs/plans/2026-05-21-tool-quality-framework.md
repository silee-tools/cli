# 도구 품질 최소 기준 framework 구현 plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 본 모노레포의 각 도구(`apps/<tool>/`)가 충족해야 할 18개 품질 dimension 을 정의하고, 그 충족 여부를 PR CI 가 hard fail 로 강제하는 framework 를 구축한다. 도구별 예외(opt-out) 는 그 도구의 PRD 에 reason 과 함께 명시한다.

**Architecture:** 본 저장소의 결정적(deterministic) 검증만 사용한다. LLM 평가·cron·로컬 도구는 도입하지 않는다. 18개 dimension 중 10개는 파일 존재·grep 으로, 5개는 도구의 `--help` 출력 파싱으로, 3개는 실제 빌드·테스트 실행으로 검증한다. opt-out 은 각 도구의 `apps/<tool>/PRD.md` 의 "품질 dimension 선언" 섹션에서 한다.

**Tech Stack:** POSIX shell (`#!/bin/sh` + `set -eu`), `go test`, `golangci-lint`, GitHub Actions, `git clone` (cross-repo formula 검증용).

---

## 1. 영구 근거와 설계 결정

본 섹션은 모든 task 가 일관된 판단을 갖도록 본 framework 의 핵심 결정들을 한 곳에 모아 둔다. 5년 뒤 본인이 이 plan 한 파일만 읽어도 의도가 복원되도록 자체완결성을 유지한다.

### 1.1 LLM 평가를 도입하지 않는다

본 framework 의 모든 검증은 결정적(deterministic) 이어야 한다. 같은 입력에 항상 같은 출력. 이유는 다음과 같다. 본 framework 의 일차 사용자는 에이전트(Claude 등) 이고, 일차 강제 지점은 PR CI 다. CI 가 LLM 호출을 포함하면 (1) API 키와 비용 부담, (2) 같은 PR 이 매번 다른 결과를 낼 위험, (3) 네트워크 의존이 함께 들어온다. 본 framework 는 그 비용을 회피하고, "이 정도로 깊은 의미적 검증은 LLM 외엔 잡기 어렵다" 가 명백한 영역(코드 의미적 동등성, 테스트가 진짜 의도를 검증하는지 등) 은 단일 `single_source_no_drift` 원칙 항목으로 떼어내 에이전트의 자체 판단에 맡긴다. 결정적 검증과 원칙 판단의 두 층 모델이다.

### 1.2 hard fail + opt-out 으로 강제한다

dimension 충족 실패는 PR CI 에서 0이 아닌 종료 코드로 PR 을 차단한다. warning-only 가 아니다. 의도된 예외는 도구의 `apps/<tool>/PRD.md` 의 품질 dimension 선언 섹션에 `opt-out` 으로 명시하고 그 이유를 한 줄로 기록한다. opt-out 으로 표시된 dimension 은 CI 가 그 도구에 대해 검증을 건너뛰며, 평가표에는 그 사실이 그대로 표시된다. 이 모델은 "예외가 있는 도구는 예외가 있다는 사실이 그 도구의 PRD 에 영구히 기록된다" 라는 자체완결성을 보장한다.

### 1.3 `--help` 출력을 도구의 단일 진실 소스로 삼는다

자동완성·README usage·plugin shell 함수 같은 보조 surface 들이 메인 기능을 팔로업하는지 검증하려면, 어디가 진실 소스인지를 정해야 한다. 본 framework 는 도구의 `--help` 출력(또는 `-h` 와 동등) 을 진실 소스로 정한다. 거기서 추출한 subcommand 와 플래그 목록이 자동완성·README 에 모두 등장하는지를 결정적으로 비교한다. 이를 가능하게 하려면 모든 도구의 `--help` 가 동일한 형식을 따라야 하며, 본 framework 는 별도의 `help_format_standard` dimension 으로 그 형식을 강제한다(1.4).

### 1.4 `--help` 표준 형식

본 저장소의 모든 도구는 `--help` 출력이 다음 구조를 따른다.

```
Usage: <tool> [...]

<도구 한 줄 설명>

Commands:                       # subcommand 있는 도구만
  <command>             <설명>
  ...

Options:
  -<short>, --<long>    <설명>
  ...

Examples:                       # 선택
  <tool> ...
```

검증 규칙:
- 첫 줄이 정확히 `Usage:` 로 시작
- `Usage:` 다음에 빈 줄, 한 줄짜리 도구 설명, 빈 줄
- `Commands:` 또는 `Options:` 헤딩이 등장(둘 중 하나는 반드시, 둘 다 가능)
- 각 entry 는 정확히 2칸 들여쓰기 + 이름 + 2칸 이상의 공백 + 설명
- `Examples:` 는 선택

flag-only 도구(예: git-tidy) 는 `Commands:` 섹션 없이 `Options:` 만 있어도 된다.

### 1.5 cross-repo 검증 범위

`formula_install_completeness` 한 dimension 만 외부 저장소(`silee-tools/homebrew-tap`) 를 읽는다. CI 에서 tap repo 를 `git clone` 으로 가져와 `Formula/<tool>.rb` 를 파싱해 `bin.install`, `bin.install_symlink`, `zsh_completion.install`, `bash_completion.install`, plugin install 등이 도구가 ship 하는 모든 surface 와 일치하는지 확인한다. tap repo 가 비공개여서 clone 이 막히면 본 dimension 은 자동 skip 하지 않고 명시적으로 opt-out 표기를 요구한다.

### 1.6 테스트 퀄리티 = (i)-(iv) 의 mechanical 합

`test_quality` dimension 은 다음 4가지 mechanical 지표로 환원된다.

- (i) `mise run test` 가 0 종료
- (ii) `go test -cover ./...` 의 패키지별 또는 전체 커버리지가 임계치 이상
- (iii) `.golangci.yml` 이 있으면 `golangci-lint run` 가 0 종료
- (iv) 테스트 파일 내 `t.Skip` / `t.SkipNow` 호출 개수가 임계치 이하

(v) "테스트 명명·구조가 좋은가" 나 (vi) "mock 남용 없는가" 같은 의미적 판단은 본 framework 의 결정적 검증 범위 밖이며, `single_source_no_drift` 와 함께 원칙 항목으로 둔다.

커버리지 임계치는 framework 구현 첫 단계에서 각 도구의 현재 커버리지를 측정한 뒤 그 값에서 -10% 마진을 두고 시작한다(과거 점수 회귀 방지). 이후 시간을 두고 단계적으로 올린다.

### 1.7 도구 자체 평가(dogfooding) 는 다음 버전에

본 framework 의 첫 버전은 도구만 평가하고 framework 자체(scripts/, workflow) 는 평가하지 않는다. 다음 버전에서 framework 의 scripts 와 docs 가 자기 자신의 dimension 을 만족하는지를 평가하는 dogfooding 을 추가한다.

---

## 2. dimension 목록과 rubric (18 + 1 = 19)

각 dimension 은 ID, 한 줄 정의, pass/fail 판정 기준, 검증 도구로 구성된다.

| ID | 정의 | pass 기준 | 검증 도구 |
|---|---|---|---|
| `version_line` | `-v`, `--version` 출력이 `<tool> v<version> © 2026 silee-tools` 한 줄 | 기존 `scripts/check-version-format.sh` 통과 | (기존) version-conformance.yml |
| `zsh_completion` | `apps/<tool>/completions/_<tool>` 존재 + `#compdef <tool>` 첫 줄 | 파일 존재 + 첫 줄 grep | `check-tool-quality.sh` |
| `bash_completion` | `apps/<tool>/completions/<tool>.bash` 존재 + `complete -F <fn> <tool>` 호출 | 파일 존재 + grep | `check-tool-quality.sh` |
| `zsh_shell_integration` | `apps/<tool>/plugin/<tool>.plugin.zsh` 존재 또는 `<tool> init zsh` 가 zsh 함수를 emit | 파일 존재 OR (도구 빌드 후) init zsh 출력에 `function ` 또는 `() {` | `check-tool-quality.sh` |
| `bash_shell_integration` | `apps/<tool>/plugin/<tool>.plugin.bash` 존재 또는 `<tool> init bash` 가 bash 함수를 emit | 파일 존재 OR (도구 빌드 후) init bash 출력에 `() {` | `check-tool-quality.sh` |
| `readme_install` | README 에 install 또는 설치 섹션 존재 | `grep -E '^##.*(install|설치)'` | `check-tool-quality.sh` |
| `readme_usage` | README 에 usage 또는 사용 섹션 존재 | `grep -E '^##.*(usage|사용)'` | `check-tool-quality.sh` |
| `unit_tests_exist` | Go 도구: `*_test.go` 1개 이상, zsh 도구: `tests/` 디렉토리 또는 동등 | `find ... -name '*_test.go'` 또는 도구별 정의 | `check-tool-quality.sh` |
| `ci_workflow` | `.github/workflows/<tool>-ci.yml` 존재 + paths 필터에 `apps/<tool>/**` 포함 | 파일 존재 + grep | `check-tool-quality.sh` |
| `goreleaser` | `apps/<tool>/.goreleaser.yaml` 존재 + `project_name: <tool>` | 파일 존재 + grep | `check-tool-quality.sh` |
| `completion_covers_help` | `<tool> --help` 에서 추출한 모든 subcommand 와 플래그가 `_<tool>` 과 `<tool>.bash` 양쪽에 등장 | 도구 빌드 → `--help` 파싱 → completion 파일 grep | `check-completion-parity.sh` |
| `readme_mentions_main_features` | `<tool> --help` 의 모든 subcommand 가 README 의 usage 섹션에서 1회 이상 언급 | `--help` 파싱 → README usage 섹션 grep | `check-completion-parity.sh` |
| `plugin_emits_main_commands` | 도구가 `init` subcommand 를 가지면 init 출력이 도구의 모든 호출형(예: jg + jgw) 을 shell 함수로 emit | `init zsh` 실행 → `() {` 함수 정의에서 함수 이름 추출 → 도구가 ship 하는 binary/symlink 목록과 비교 | `check-completion-parity.sh` |
| `goreleaser_archive_completeness` | `apps/<tool>/.goreleaser.yaml` 의 `archives.files` 가 `completions/` 와 `plugin/` 디렉토리(있으면) 의 모든 파일 패턴을 포함 | YAML 파싱 + 디렉토리 비교 | `check-tool-quality.sh` |
| `formula_install_completeness` | homebrew-tap 의 `Formula/<tool>.rb` `install` 블록이 도구의 binary(들) + 모든 completion + 모든 plugin 파일을 install | tap repo clone → formula 파싱 → 도구 surface 와 비교 | `check-formula-install.sh` |
| `tests_execute_and_pass` | `cd apps/<tool> && mise run test` 가 0 종료 | 명령 실행 | `check-test-quality.sh` |
| `test_quality` | (i) tests pass + (ii) coverage ≥ 임계치 + (iii) linter (`.golangci.yml` 있으면) 통과 + (iv) `t.Skip` 호출 임계치 이하 | 4 개 sub-check | `check-test-quality.sh` |
| `help_format_standard` | `<tool> --help` 출력이 1.4 의 표준 형식을 따른다 | 도구 빌드 → `--help` 출력 grep | `check-help-format.sh` |
| (m) `single_source_no_drift` | 같은 내용이 두 곳 이상에 나타나면 한쪽이 derived 이고 한쪽이 source 다. 두 곳에 같은 텍스트를 별도로 쓰지 않는다. 의미적 검증으로 본 framework 의 결정적 자동 검증 대상이 아니다 | (수동/에이전트 판단) | 본 항목은 CLAUDE.md 에 원칙으로만 명시 |

(m) 은 mechanical 검증 없이 원칙 항목으로만 유지되며, 평가표에는 별도 카테고리로 표시(예: "원칙 항목 — CI 검증 없음"). 다른 18 개는 모두 hard fail.

---

## 3. 파일 구조

본 framework 구현 후 cli 저장소의 신규/변경 파일은 다음과 같다.

```
cli/
├── docs/
│   └── quality-checklist.md          # 신규: 사람이 읽는 dimension + rubric (본 plan 의 §2 를 정리한 문서)
├── apps/
│   ├── jg/PRD.md                     # 신규: 사용자가 채우는 PRD + dimension 선언
│   ├── totp/PRD.md                   # 신규: 동일
│   └── git-tidy/PRD.md               # 신규: 동일
├── scripts/
│   ├── check-tool-quality.sh         # 신규: 메인 runner. 도구 한 개를 입력으로 받고 모든 dimension 을 평가
│   ├── check-help-format.sh          # 신규: --help 표준 형식 검증
│   ├── check-completion-parity.sh    # 신규: --help 출력 파싱 + completion/README 와 매칭
│   ├── check-test-quality.sh         # 신규: coverage + linter + skip count
│   ├── check-formula-install.sh      # 신규: tap repo clone + formula install 블록 검증
│   └── extract-tool-symbols.sh       # 신규: 도구의 --help 에서 subcommand 와 플래그 추출. 다른 스크립트가 호출
├── .github/workflows/
│   └── tool-quality.yml              # 신규: PR CI workflow
└── CLAUDE.md                         # 갱신: 5번째 원칙 (선택 — 본 framework 자체가 원칙) + single_source_no_drift 원칙 명시
```

---

## 4. 도구별 PRD 와 본 framework 가 만나는 지점

각 `apps/<tool>/PRD.md` 의 마지막 섹션 "품질 dimension 선언" 은 다음 형식의 표를 갖는다(템플릿 별도 commit).

| dimension | 상태 | reason (opt-out 인 경우) |
|---|---|---|
| version_line | opt-in | |
| zsh_completion | opt-in 또는 opt-out | 비어 있으면 opt-in. opt-out 이면 reason 필수 |
| ... | ... | ... |

`check-tool-quality.sh` 가 본 표를 파싱해 opt-out dimension 을 건너뛴다. 표 형식 파서는 markdown table 가정. dimension 이름은 §2 와 동일. opt-out 인데 reason 칸이 비어 있으면 schema 위반으로 hard fail.

---

## 5. 작업 단위

각 task 는 별 commit. TDD 사이클(가능한 경우 Red→Green→Refactor).

### Task 1: dimension 정의 문서

**Files:**
- Create: `docs/quality-checklist.md`

본 plan 의 §1~§2 를 사람 친화 문서로 정리한다. 향후 변경의 단일 진실 소스가 본 문서이며, plan 은 historical record 로 남는다.

### Task 2: 도구 symbol 추출기

**Files:**
- Create: `scripts/extract-tool-symbols.sh`
- Create: `scripts/test/fixtures/sample-help-output.txt` (테스트 fixture)
- Create: `scripts/test/extract-tool-symbols.test.sh` (POSIX shell 테스트)

`<tool> --help` 출력을 받아 subcommand 와 플래그 한 줄씩 출력. §1.4 의 표준 형식 가정. 테스트 fixture 로 jg/totp/git-tidy 의 실제 `--help` 출력을 미리 캡처해 두고 비교.

### Task 3: `help_format_standard` 검증

**Files:**
- Create: `scripts/check-help-format.sh`

§1.4 의 표준 형식을 grep + awk 로 검증. 도구 빌드 후 `--help` 실행 → 형식 검증 → 0/non-0 종료.

### Task 4: `completion_covers_help`, `readme_mentions_main_features`, `plugin_emits_main_commands`

**Files:**
- Create: `scripts/check-completion-parity.sh`

extract-tool-symbols.sh 가 추출한 symbol 들을 (1) `apps/<tool>/completions/_<tool>` grep, (2) `apps/<tool>/completions/<tool>.bash` grep, (3) README usage 섹션 grep 으로 매칭. 누락 시 fail 라인 출력. plugin_emits_main_commands 는 `init zsh` 출력에서 함수 정의 라인 grep.

### Task 5: 메인 quality runner

**Files:**
- Create: `scripts/check-tool-quality.sh`

도구 한 개(첫 인자) 를 받아 모든 18 dimension 을 평가. opt-out 인 dimension 은 skip 표기. 결과를 평가표 형식으로 stdout. 한 dimension 이라도 fail 이면 종료 코드 1. PRD 의 opt-out 선언 파서는 markdown table 가정.

### Task 6: `test_quality` 검증

**Files:**
- Create: `scripts/check-test-quality.sh`

(i) `mise run test`, (ii) `go test -cover` 로 측정한 패키지별 커버리지 vs 임계치, (iii) `golangci-lint run` (.golangci.yml 있으면), (iv) `grep -r 't\.Skip' apps/<tool>/' 개수. 임계치는 환경 변수 `JG_COVERAGE_THRESHOLD=70` 같은 형태로 도구별 override 가능.

### Task 7: 현재 커버리지 측정 + 임계치 설정

**Files:**
- Modify: `apps/<tool>/PRD.md` 의 dimension 선언 표에 `test_quality` reason 또는 임계치 메모 추가

`cd apps/jg && go test -cover ./...` 등으로 각 Go 도구의 현재 커버리지를 측정해 본 plan 의 후속 섹션에 기록. 임계치는 측정값 - 10% 로 시작. 측정 결과는 `docs/quality-checklist.md` 의 "현재 임계치" 표에 기록.

### Task 8: `formula_install_completeness` 검증

**Files:**
- Create: `scripts/check-formula-install.sh`

`git clone --depth=1 https://github.com/silee-tools/homebrew-tap /tmp/tap-snapshot` → `Formula/<tool>.rb` 파싱 → install 블록의 `bin.install`, `bin.install_symlink`, `zsh_completion.install`, `bash_completion.install`, plugin install 모두 추출 → 도구의 실제 surface(completions/*, plugin/*, 바이너리 이름, symlink) 와 매칭.

### Task 9: 도구별 보완 — totp

**Files:**
- Create: `apps/totp/completions/_totp`
- Create: `apps/totp/completions/totp.bash`
- Modify: `apps/totp/.goreleaser.yaml` (archive 에 `completions/*` 추가)
- Modify: `apps/totp/PRD.md` (dimension 선언 갱신)

totp 의 자동완성 신규 작성(macOS Keychain 항목명을 동적 후보). totp 는 shell init 명령이 없으므로 zsh_shell_integration, bash_shell_integration, plugin_emits_main_commands 는 opt-out (PRD 에 reason 명시).

### Task 10: 도구별 보완 — git-tidy

**Files:**
- Modify: `apps/git-tidy/PRD.md`

git-tidy 는 zsh plugin 자체이고 Go 바이너리가 없다. PRD 에 다음을 opt-out 으로 선언:
- bash_completion (zsh 전용)
- bash_shell_integration (zsh 전용)
- completion_covers_help (Go 바이너리 없음. --help 는 zsh 함수가 출력하므로 별 검증 경로 필요할 수 있음)
- plugin_emits_main_commands (`init` subcommand 없음)
- test_quality 중 (ii) coverage / (iii) golangci-lint (Go 가 아님) — 또는 zsh 전용 측정 방법 정의
- formula_install_completeness 의 zsh_completion 항목 (필요 시)

zsh-only 도구의 검증 정책을 plan 본 단계에서 한 번 더 합의한다.

### Task 11: 도구별 보완 — jg (마무리 점검)

**Files:**
- Modify: `apps/jg/PRD.md` (dimension 선언 — 거의 전부 opt-in)
- Modify: 필요 시 `apps/jg/cmd/jg/main.go` 의 `printHelp` 가 표준 형식인지 확인

jg 는 현재 가장 완비된 도구. PRD 작성 후 check-tool-quality.sh 가 jg 에 대해 그린 라이트인지 검증. red 면 fix.

### Task 12: CI workflow

**Files:**
- Create: `.github/workflows/tool-quality.yml`

trigger: paths `apps/**`, `docs/quality-checklist.md`, `scripts/check-*.sh`. 각 도구마다 `scripts/check-tool-quality.sh <tool>` 실행. tap repo 는 anonymous clone 가능(public). 한 도구라도 fail 이면 workflow fail.

### Task 13: 루트 CLAUDE.md 갱신

**Files:**
- Modify: `CLAUDE.md` (저장소 원칙에 `single_source_no_drift` 원칙 명시 + 본 framework 가 새 도구에 자동 적용된다는 짧은 안내)

원칙으로 single_source_no_drift 한 줄 추가:
> **단일 진실 소스 (no drift)**: 같은 내용이 두 곳 이상에 나타나면 한쪽을 derived 로 만들고 lock test 또는 embed 로 분기를 차단한다. 의미적 정합성은 결정적 검증으로 잡기 어려우므로 에이전트가 작업 시 의식적으로 적용한다.

신규 도구 체크리스트에 한 줄 추가: "도구 PRD 작성(`apps/<new-tool>/PRD.md`) + 품질 dimension 선언".

---

## 6. 수용 기준

- `scripts/check-tool-quality.sh jg` 가 0 종료
- `scripts/check-tool-quality.sh totp` 가 0 종료 (opt-out 으로 처리된 dimension 제외)
- `scripts/check-tool-quality.sh git-tidy` 가 0 종료 (opt-out 으로 처리된 dimension 제외)
- 의도적 위반 (예: jg 의 `_jg` 에서 한 subcommand 제거) PR 이 CI 에서 hard fail
- 각 `apps/<tool>/PRD.md` 가 "품질 dimension 선언" 섹션을 갖고 있음
- 루트 `CLAUDE.md` 가 `single_source_no_drift` 원칙을 포함

---

## 7. 본 plan 범위 밖

다음은 본 plan 의 첫 버전이 다루지 않으며, 추후 별 plan 에서 다룬다.

- LLM 평가 (semantic test quality, single_source_no_drift 의 자동 검증)
- cron / scheduled re-evaluation
- 평가 결과의 누적 추적 (scorecard 파일을 commit 으로 누적)
- 점수 회귀 알림 봇
- framework 자체에 대한 평가 (dogfooding)
- 도구 PRD 의 자동 일관성 검증 (서로 다른 PRD 간 contradicting claims 등)
- tap repo 에 대한 PR 자동 갱신 (예: 새 completion 파일 추가 시 formula 갱신 PR 자동 생성)
