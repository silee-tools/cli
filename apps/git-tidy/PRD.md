# git-tidy PRD

## 한 줄 정의

git-tidy 는 로컬 git 브랜치를 일괄 정리하는 명령줄 도구다. 작업이 끝났거나 오래
방치된 브랜치를 자동으로 찾아 보여주고, 사용자가 체크박스로 확인하면 한 번에
삭제한다.

## 대상 사용자

로컬 브랜치가 쌓이는 것을 싫어하는 git 사용자다. 작업이 끝난 브랜치가 로컬에 계속
남아 목록을 어지럽히는 것을 주기적으로 정리하고 싶은 사용자를 상정한다.

## 목표

- **하이브리드 정리 모델로 안전하게**: 삭제 후보 신호(작업이 끝났거나 낡았다는 양성
  근거)와 보호 규칙(절대 건드리면 안 되는 구조적 조건) 두 층을 조합한다. 신호에
  걸리고 보호 규칙에 걸리지 않은 브랜치만 정리 대상이 된다.
- **기본 dry-run 으로 실수 방지**: 인자 없이 실행하면 삭제 대상만 보여주고 실제로는
  지우지 않는다. 실제 삭제는 사용자가 명시적으로 요청할 때만 일어난다.
- **다중 선택 확인으로 한 번 더**: `--run` 실행 시 삭제 대상을 체크박스로 보여줘
  사용자가 최종 확정한 브랜치만 삭제한다.
- **자동완성 최대 제공**: 플래그 같은 정적 후보를 zsh·bash 자동완성으로 제공한다.

## 비목표

- **원격 브랜치 삭제**: git-tidy 는 로컬 브랜치만 정리한다. 원격(remote) 브랜치를
  삭제하지 않는다.
- **브랜치 생성·이름 변경**: 브랜치를 만들거나 이름을 바꾸는 기능은 제공하지 않는다.
  정리 한 가지만 담당한다.
- **머지·rebase 같은 git 작업**: 머지·rebase·cherry-pick 같은 작업을 대행하지
  않는다.

## 기능 범위

- `git-tidy`: dry-run 으로 동작한다. 삭제 대상과 제외된 후보를 표시하고 실제로
  삭제하지 않는다. 인자 없는 기본 동작이다.
- `git-tidy --run`: 삭제 대상을 다중 선택 화면에 띄워, 사용자가 확정한 브랜치만
  삭제한다. 기본은 체크박스 TUI 다.
- `git-tidy --run --no-tui`: 체크박스 TUI 대신 줄 기반 선택을 강제한다.
- `git-tidy --stale-days=N`: stale 판정 창을 N일로 바꾼다. 기본값은 20일이며
  환경변수 `GIT_TIDY_STALE_DAYS` 로도 설정할 수 있다.
- `git-tidy --no-fetch`: `git fetch --prune` 단계를 건너뛴다. 오프라인이거나 원격이
  없는 환경에서 사용한다.
- `git-tidy --version` / `git-tidy -v`: 버전을 출력한다.
- `git-tidy --help` / `git-tidy -h`: 사용법을 출력한다.
- 삭제 후보 신호: 다음 세 신호 중 하나라도 해당하면 그 브랜치가 삭제 후보가 된다.
  (1) upstream 추적 브랜치가 사라진 `[gone]` 상태, (2) base 브랜치에 이미 머지된
  상태, (3) 마지막 커밋 또는 merge-base 가 stale 판정 창보다 오래된 stale 상태.
- 보호 규칙: 후보로 잡혔더라도 다음 중 하나라도 해당하면 삭제하지 않는다. (1) 현재
  체크아웃된 브랜치, (2) 자동 감지한 base 브랜치(main·master·trunk 등).

## 주요 시나리오

1. **dry-run 으로 삭제 대상 확인**: 사용자가 `git-tidy` 를 실행하면 삭제 대상과
   제외된 후보가 표시되고 실제 삭제는 일어나지 않는다. 사용자는 무엇이 지워질지
   먼저 확인한다.
2. **실제 정리 실행**: 사용자가 `git-tidy --run` 을 실행하면 삭제 대상을 체크박스로
   확인하고, 최종 확정한 브랜치만 삭제한다.
3. **stale 브랜치 정리**: 오래 방치된 브랜치가 stale 신호로 삭제 후보에 오른다.
   `--stale-days=N` 으로 판정 창을 조절할 수 있다.

## 수용 기준

- 인자 없는 `git-tidy` 호출은 dry-run 으로 동작해 삭제 대상과 제외된 후보를 표시하고
  브랜치를 건드리지 않는다. 실제 삭제는 `--run` 을 명시할 때만 일어난다.
- 삭제 후보 신호(`[gone]` / merged / stale) 중 하나라도 해당하는 브랜치가 삭제
  후보가 된다. 보호 규칙(현재 브랜치, base 브랜치)에 해당하는 후보는 제외된다.
- `--run` 실행 시 삭제 대상을 다중 선택 화면에 보여주며, 사용자가 확정한 브랜치만
  삭제한다. 취소하거나 전부 해제하면 아무것도 지우지 않는다.
- 같은 저장소 상태에서 git-tidy 를 여러 번 실행해도 추가 부작용 없이 같은 결과가
  나온다.

## 외부 의존

- **git**: 브랜치 조회, `git fetch --prune`, 브랜치 삭제에 사용한다. 필수 의존이다.
- **원격 저장소**: `git fetch --prune` 단계에 필요하다. `--no-fetch` 로 이 단계를
  건너뛸 수 있으므로 선택적 의존이다.
- **zsh**: zsh 자동완성(`completions/_git-tidy`) 을 사용하는 경우에만 필요하다.
  선택적 의존이다.
- **bash**: bash 자동완성(`completions/git-tidy.bash`) 을 사용하는 경우에만 필요하다.
  선택적 의존이다.

## 품질 dimension 선언

본 도구는 `docs/plans/2026-05-21-tool-quality-framework.md` 의 18개 dimension 에 대해
다음과 같이 선언한다. opt-out 인 항목은 reason 한 줄 필수.

| dimension | 상태 | reason (opt-out 시) |
|---|---|---|
| version_line | opt-in | |
| zsh_completion | opt-in | |
| bash_completion | opt-in | |
| zsh_shell_integration | opt-out | 셸 상태를 바꾸지 않는 순수 CLI 라 셸 통합이 없다 |
| bash_shell_integration | opt-out | 셸 상태를 바꾸지 않는 순수 CLI 라 셸 통합이 없다 |
| readme_install | opt-in | |
| readme_usage | opt-in | |
| unit_tests_exist | opt-in | |
| ci_workflow | opt-in | |
| goreleaser | opt-in | |
| completion_covers_help | opt-in | |
| readme_mentions_main_features | opt-in | |
| plugin_emits_main_commands | opt-out | git-tidy 는 init subcommand 가 없어 plugin emit 대상이 아니다 |
| goreleaser_archive_completeness | opt-in | |
| formula_install_completeness | opt-in | |
| tests_execute_and_pass | opt-in | |
| test_quality | opt-in | |
| help_format_standard | opt-in | |
