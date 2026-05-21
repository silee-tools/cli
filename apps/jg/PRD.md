# jg PRD

## 한 줄 정의

jg 는 git 저장소와 그 worktree 들 사이를 자주 이동하는 개발자를 위한 점프 도구다.
단일 바이너리가 호출 이름에 따라 두 모드로 동작한다. `jg` 로 호출하면 git 저장소
사이를, `jgw` 로 호출하면 한 저장소의 원본 working tree 와 linked worktree 사이를
점프한다.

## 대상 사용자

fzf 와 zsh 또는 bash 를 사용하는 macOS·Linux 개발자다. 여러 git 저장소와
worktree 를 오가며 터미널 중심으로 작업하는 사용자를 상정한다. 1인 운영 도구이지만
설치 절차와 문서는 다른 사용자가 그대로 따라 할 수 있는 수준을 유지하며, 그에 따른
셸 호환성과 배포 호환성을 함께 고려한다.

## 목표

- **frecency 기반 우선순위 제시**: 단순 최근 방문이나 단순 방문 빈도가 아니라,
  방문 빈도와 최근성을 함께 반영한 점수로 후보를 정렬한다. 자주 그리고 최근에 간
  곳일수록 picker 의 위쪽에 온다.
- **cwd 무관 어디서나 호출 가능**: 현재 작업 디렉토리가 git 저장소 안이든 밖이든
  동일하게 점프 기능을 제공한다. 사용자가 호출 위치를 의식할 필요가 없다.
- **자동완성 최대 제공**: subcommand 와 플래그 같은 정적 후보뿐 아니라 저장소
  목록·worktree 목록 같은 동적 후보까지, zsh 와 bash 양쪽 셸이 표현할 수 있는
  범위까지 자동완성을 제공한다.

## 비목표

- **worktree·repo 라이프사이클 관리**: worktree 와 저장소의 생성·삭제·이름 변경은
  다루지 않는다. 이는 `git worktree add` 같은 git 본체의 책임이며, jg 는 이미
  존재하는 대상으로의 이동만 담당한다.
- **비-git 디렉토리 점프**: z·autojump 처럼 임의의 디렉토리로 점프하는 기능은
  제공하지 않는다. 점프 대상은 git 저장소와 그 worktree 로 한정한다.
- **git 명령 래핑**: commit·branch·pull 같은 git 작업을 감싸거나 대행하지 않는다.
  jg 의 책임은 이동 한 가지다.

## 기능 범위

### 핵심 기능

- `jg`: fzf picker 로 git 저장소를 interactive 하게 골라 점프한다.
- `jg <query>`: query 로 후보를 미리 좁힌 뒤 점프한다.
- `jgw`: 현재 작업 디렉토리를 기준으로 자동 분기한다. 디렉토리가 git 저장소 안이면
  그 저장소의 worktree picker 한 단계를, 밖이면 저장소 picker 와 worktree picker
  의 두 단계를 띄운다.
- `jgw <pattern>`: 저장소 후보를 pattern 으로 좁힌 뒤 두 단계 흐름으로 진행한다.
- `jg init <shell>`: zsh 또는 bash 용 셸 통합 코드를 표준 출력으로 내보낸다. 이
  출력은 `jg` 와 `jgw` 두 셸 함수를 함께 정의한다.
- `jg setup`: 사용자의 셸을 감지해 셸 통합을 자동으로 등록한다.
- `jg --add <path>` / `jg --remove <path>`: frecency 항목을 수동으로 추가하거나
  제거한다.
- `jg -l` / `jg --list`: frecency 점수와 함께 추적 중인 저장소 목록을 출력한다.
- `jg clean` / `jg --clean`: 더 이상 유효하지 않은 stale 항목을 정리한다.
- frecency store: XDG Base Directory 표준 경로 아래에 저장소 점수와 worktree 점수를
  각각 별 파일로 보관한다.
- zsh·bash 자동완성: 두 셸 모두에 정적·동적 후보 자동완성을 제공한다.
- 방문 자동 기록: zsh 의 chpwd hook 과 bash 의 PROMPT_COMMAND hook 으로 사용자가
  방문한 git 저장소를 자동으로 frecency store 에 기록한다.

### 부가 기능

- `jg scheduler <install|remove|status>`: stale 항목을 주기적으로 정리하는 일일
  스케줄러를 운영체제에 등록·해제·조회한다. 점프와 frecency 라는 핵심 가치에
  직접 기여하지 않는 운영 편의 기능이므로 부가 기능으로 둔다.

## 주요 시나리오

1. **자주 가던 저장소로 점프**: 사용자가 터미널 어디에서나 `jg` 를 입력하면
   frecency 로 정렬된 picker 가 뜨고, 후보를 골라 그 저장소로 이동한다. 저장소
   이름의 일부만 기억날 때는 query 를 함께 입력해 후보를 좁힌다.
2. **한 저장소의 worktree 들 사이 이동**: 사용자가 어떤 저장소 안에서 `jgw` 를
   입력하면 그 저장소의 원본 working tree 와 linked worktree 들이 picker 에 나오고,
   고른 worktree 로 이동한다.
3. **저장소 밖에서 worktree 로 바로 점프**: 사용자가 git 저장소 바깥에서 `jgw` 를
   입력하면 먼저 저장소 picker 가, 저장소를 고르면 그 저장소의 worktree picker 가
   이어서 뜬다. 두 단계를 거쳐 원하는 worktree 로 이동한다.

## 수용 기준

- jg 가 frecency 로 정렬된 후보를 picker 로 보여주고, 사용자가 후보를 선택하면 셸
  함수가 그 경로로 이동한다.
- jgw 가 현재 작업 디렉토리가 git 저장소 안인지 밖인지에 따라 흐름을 자동으로
  분기하며, 각 흐름이 명세대로 동작한다.
- frecency store 가 XDG_STATE_HOME 표준 경로 아래에 자리하며, 기존 경로를 쓰던
  사용자가 별도 조작 없이 투명하게 마이그레이션된다.
- jg 와 jgw 의 자동완성과 셸 함수 통합이 zsh 와 bash 양쪽에서 동일하게 동작한다.

## 외부 의존

- **git**: worktree 목록 조회, 저장소 식별, 브랜치 조회에 사용한다. 필수 의존이다.
- **fzf**: picker 사용자 인터페이스에 사용한다. fzf 가 없으면 picker 기능은 오류로
  종료하며 별도 fallback 을 제공하지 않는다. 필수 의존이다.
- **zsh 또는 bash**: 점프 결과를 현재 셸의 작업 디렉토리 변경으로 반영하려면 셸
  함수 통합이 필요하다. jg 가 지원하는 셸은 zsh 와 bash 다.

## 품질 dimension 선언

본 도구는 `docs/plans/2026-05-21-tool-quality-framework.md` 의 18개 dimension 에 대해
다음과 같이 선언한다. opt-out 인 항목은 reason 한 줄 필수.

| dimension | 상태 | reason (opt-out 시) |
|---|---|---|
| version_line | opt-in | |
| zsh_completion | opt-in | |
| bash_completion | opt-in | |
| zsh_shell_integration | opt-in | |
| bash_shell_integration | opt-in | |
| readme_install | opt-in | |
| readme_usage | opt-in | |
| unit_tests_exist | opt-in | |
| ci_workflow | opt-in | |
| goreleaser | opt-in | |
| completion_covers_help | opt-in | |
| readme_mentions_main_features | opt-in | |
| plugin_emits_main_commands | opt-in | |
| goreleaser_archive_completeness | opt-in | |
| formula_install_completeness | opt-in | |
| tests_execute_and_pass | opt-in | |
| test_quality | opt-in | |
| help_format_standard | opt-in | |
