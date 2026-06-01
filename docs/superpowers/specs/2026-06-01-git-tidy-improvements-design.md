# git-tidy 개선 설계

로컬 git 브랜치 정리 도구 git-tidy 의 삭제 후보 제시 방식과 선택 화면을 개선한다.
네 가지를 다룬다: (1) 확실한 후보만 기본 선택, (2) 삭제 사유별 정렬, (3) 그룹 헤더와
그룹 일괄 토글을 갖춘 강화된 TUI, (4) 브랜치별 worktree 이름과 stale 경과 일수 표시.

## 목표

- 기본 선택을 가장 확실한 후보(`[gone]`)로 한정해, 사용자가 무심코 전체를 삭제하는
  사고를 줄인다.
- 삭제 후보를 삭제 사유(신호)별로 묶어 보여주어, 사용자가 사유 단위로 판단·선택하게 한다.
- 선택 화면의 가독성과 조작성을 높인다(색, 그룹 헤더, 그룹 일괄 토글, 경과 정보).

## 비목표

- 삭제 후보 판정 로직(gone / merged / stale 신호와 보호 규칙) 자체는 바꾸지 않는다.
- 원격 브랜치 정리, reflog 기반 복구 같은 새 기능은 다루지 않는다.

## 신호와 용어

- `gone`: upstream 추적 브랜치가 사라진 상태. PR 머지 후 원격 브랜치가 삭제되면 발생.
- `merged`: `git branch --merged <base>` 로 확인된, base 에 이미 합쳐진 브랜치.
- `stale`: 마지막 커밋 또는 merge-base 가 stale 판정 창(기본 20일)보다 오래된 브랜치.

신호 순위는 `gone`=0 → `merged`=1 → `stale`=2 로 정한다(확실한 순).

## 1. 데이터 모델과 정렬 (classify 패키지)

정렬은 "삭제 사유"라는 도메인 판단이므로 순수 함수인 `classify` 에서 수행한다. dry-run
출력·TUI·줄 모드가 모두 같은 순서를 공유하고, 정렬 로직이 단위 테스트 가능한 한 곳에만
존재하게 하기 위함이다.

- `Classify` 가 `ToDelete` 를 만든 뒤 신호 순위로 1차 정렬하고, 같은 그룹 안에서는
  브랜치 이름으로 2차 정렬한다.
- `classify.Result` 에 `AgeDays int` 필드를 추가한다. `stale` 후보일 때 마지막 커밋
  시각(`CommitUnix`) 기준 경과 일수 `(Now - CommitUnix) / 86400` 를 채운다. 커밋 시각
  정보가 없으면(`CommitUnix == 0`) merge-base 시각으로 폴백한다. `gone`·`merged`
  항목은 0(미표시).
- `WorktreePath` 는 기존 그대로 유지하며, 표시 단계에서 디렉터리 베이스 이름을 쓴다.

## 2. 선택 모델 (pick 패키지)

현재 `pick.Selection` 은 평평한 문자열 목록이고 생성 시 전부 체크된다. 그룹과 항목별
초기 체크 상태를 아는 모델로 바꾼다.

- 항목 하나는 `이름 / 신호 / worktree 경로 / 경과 일수 / 체크 여부` 를 들고 있는다.
- 생성 시 `gone` 신호인 항목만 체크되고 나머지(`merged`·`stale`)는 해제된 상태로 시작한다
  (요구사항 1).
- 그룹(신호) 단위 연산을 추가한다(요구사항 2):
  - `Groups()` — 등장 순서대로 그룹 키 목록.
  - `ToggleGroup(signal)` — 그 그룹 안에 하나라도 해제된 게 있으면 전체 체크,
    다 체크돼 있으면 전체 해제. 기존 `ToggleAll` 규칙을 그룹 범위로 적용.
- 기존 `Toggle(i)` / `ToggleAll()` / `Checked()` / `IsChecked(i)` / `Items()` 는 유지한다.

이 모델은 TUI·줄 모드가 함께 쓰고 `model_test.go` 로 검증한다.

## 3. 강화된 TUI (bubbletea)

`internal/pick/tui.go` 를 `charmbracelet/bubbletea` + `lipgloss` 기반으로 재작성한다.
Model/Update/View 구조로 가고, 색·강조는 lipgloss 로 입힌다. 화면은 그룹 헤더와 항목을
한 줄씩 섞은 행 목록이며, 커서는 헤더 행과 항목 행을 모두 지난다.

### 화면 목업

```
  git-tidy — 삭제할 브랜치 선택  (1/7 선택됨)

  ▾ gone (2)                  ← upstream 사라짐 · PR 머지 후
›   ◉ feature/login      ⌂ login   [worktree 동반 제거]
    ◉ fix/typo-readme
  ▾ merged (3)                ← base 에 이미 합쳐짐
    ◯ feature/old-api
    ◯ chore/bump-deps
    ◯ docs/update-guide
  ▾ stale (2)                 ← 20일+ 경과
    ◯ spike/experiment   34일 경과
    ◯ wip/draft          51일 경과

  ↑↓/jk 이동 · space 토글 · a 전체 · enter 삭제 · esc 취소
```

### 키 동작

- `↑`/`↓`, `j`/`k`: 행 이동 (헤더·항목 모두 정지).
- `space`: 커서가 항목이면 그 항목 토글, 헤더면 그 그룹 전체 토글.
- `a`: 전체 토글.
- `enter`: 체크된 항목 삭제 확정.
- `esc`/`q`/`Ctrl-C`: 취소.

### 색·강조 (lipgloss)

- 그룹 헤더: 굵게, 신호별 색 — `gone` 빨강 계열, `merged` 초록 계열, `stale` 노랑 계열.
- 체크된 항목 `◉` 는 강조색, 해제 `◯` 는 흐리게(dim).
- 커서 행은 배경 반전 또는 굵게.
- 상단 제목 줄에 `(선택됨/전체)` 실시간 카운트, 하단에 단축키 안내.
- worktree 에 물린 브랜치는 브랜치명 뒤에 `⌂ <worktree 디렉터리 이름>` 을 흐리게 붙이고
  `[worktree 동반 제거]` 표식을 둔다.
- `stale` 항목은 라벨에 `N일 경과` 를 흐리게 붙인다.

### 긴 목록

후보가 터미널 높이를 넘으면 커서를 따라 스크롤하는 뷰포트를 둔다(`bubbles/viewport`
또는 직접 윈도잉). 짧으면 전체를 그대로 그린다.

### 비대화형 처리

bubbletea 가 TTY 가 아니면 시작에 실패한다. 이때는 줄 기반(`RunLine`)으로 폴백하고,
출력이 파이프라 둘 다 불가하면 "터미널이 필요합니다" 안내 후 종료한다(`pick.DetectMode`
세 모드 로직 유지).

## 4. 줄 모드 폴백 (`--no-tui` / TTY 아님)

강화된 모델을 그대로 공유한다. 그룹 헤더 줄과 번호 매긴 항목을 함께 출력하고, 기본 체크
차등(`gone`만 체크)·worktree 표시·stale 경과 일수를 반영한다. 토글은 번호 입력, `a`
전체 토글을 유지한다. 그룹 일괄 토글은 TUI 전용으로 두고 줄 모드에는 넣지 않는다 —
줄 모드는 최소 폴백이고, 번호 기반 입력에 그룹 명령을 얹으면 입력 규약이 복잡해지기
때문이다.

```
  ── gone (2) ──
   1. [x] feature/login      ⌂ login
   2. [x] fix/typo-readme
  ── merged (3) ──
   3. [ ] feature/old-api
  ── stale (2) ──
   6. [ ] spike/experiment   34일 경과
  번호=토글, a=전체토글, 빈 줄=완료, q=취소 >
```

## 5. dry-run 출력 (`printTargets`)

신호별로 묶어 그룹 헤더와 개수를 보여주고, worktree 동반 제거 표식과 stale 경과 일수를
유지한다. classify 가 이미 정렬해 주므로 순서대로 출력만 한다.

## 테스트 (TDD Red→Green)

- `classify_test.go`: 신호별 정렬 순서(gone→merged→stale, 그룹 내 이름순)와 `AgeDays`
  계산(커밋 시각 기준, 커밋 시각 부재 시 merge-base 폴백) 검증을 추가한다.
- `model_test.go`: 기본 체크가 `gone`만인지, `ToggleGroup` 이 그룹 범위로만 동작하는지,
  worktree·경과 일수 필드가 보존되는지 검증한다.
- `line_test.go`: 그룹 헤더 출력·기본 체크 차등·worktree·경과 일수 렌더링을 검증한다.
- bubbletea TUI 는 `Model.Update` 를 순수 함수로 분리해 키 입력→상태 전이(메시지 주입)를
  단위 테스트하고, 실제 렌더링·삭제 동작은 1회성 수동 E2E(실제 저장소에서 `gtidy!`
  실행)로 종결한다.

## 문서·의존성 갱신

- `go.mod` 에 `charmbracelet/bubbletea`·`lipgloss`(필요 시 `bubbles`) 추가.
- README 의 동작 설명(선택 화면·기본 선택 정책)과 `apps/git-tidy/CLAUDE.md` 의 정리 모델
  요약을 갱신한다.

## 외부 의존

- git CLI (기존 그대로).
- charmbracelet/bubbletea, lipgloss (신규 TUI 토대). 형제 도구(jg·totp)는 쓰지 않지만,
  git-tidy 의 선택 화면이 가장 상호작용이 많아 도입 가치가 도구 단위로 정당화된다.
