# git-tidy PRD

## 한 줄 정의

git-tidy 는 upstream 이 사라진 로컬 git 브랜치를 안전하게 정리하는 zsh 플러그인이다.
원격 저장소에서 브랜치가 삭제되어 로컬 추적 브랜치가 갈 곳을 잃은 상태가 되면, 보호
규칙을 적용한 뒤 그 브랜치들을 정리한다.

## 대상 사용자

로컬 브랜치가 쌓이는 것을 싫어하는 git 사용자다. 작업이 끝난 브랜치가 로컬에 계속
남아 목록을 어지럽히는 것을 주기적으로 정리하고 싶은 사용자를 상정한다.

## 목표

- **보호 규칙으로 안전하게 정리**: 삭제하면 안 되는 브랜치를 보호 규칙으로 정의하고,
  보호 대상은 정리에서 제외한다. 정리는 보호 규칙을 통과한 브랜치에만 적용한다.
- **기본 dry-run 으로 실수 방지**: 인자 없이 실행하면 삭제 대상만 보여주고 실제로는
  지우지 않는다. 실제 삭제는 사용자가 명시적으로 요청할 때만 일어난다.
- **자동완성 최대 제공**: 플래그 같은 정적 후보를 zsh 자동완성으로 제공한다.

## 비목표

- **원격 브랜치 삭제**: git-tidy 는 로컬 브랜치만 정리한다. 원격(remote) 브랜치를
  삭제하지 않는다.
- **브랜치 생성·이름 변경**: 브랜치를 만들거나 이름을 바꾸는 기능은 제공하지 않는다.
  정리 한 가지만 담당한다.
- **머지·rebase 같은 git 작업**: 머지·rebase·cherry-pick 같은 작업을 대행하지
  않는다.

## 기능 범위

- `git-tidy`: dry-run 으로 동작한다. 정리 대상 브랜치만 표시하고 실제로 삭제하지
  않는다. 인자 없는 기본 동작이다.
- `git-tidy --run`: 보호 규칙을 적용한 뒤 정리 대상 브랜치를 실제로 삭제한다.
- `git-tidy --days=N`: 최근 N일 이내에 커밋이 있는 브랜치를 보호한다. 기본값은
  7일이다.
- `git-tidy --no-fetch`: `git fetch --prune` 단계를 건너뛴다. 오프라인이거나 원격이
  없는 환경에서 사용한다.
- `git-tidy --version` / `git-tidy -v`: 버전을 출력한다.
- `git-tidy --help` / `git-tidy -h`: 사용법을 출력한다.
- 보호 규칙: 정리 대상에서 제외할 브랜치를 규칙으로 정의한다. 현재 체크아웃된
  브랜치, 기본 브랜치(main·master·trunk 등 자동 감지), 다른 worktree 에서 체크아웃
  중인 브랜치, 최근 보호 기간 이내에 커밋이 있는 브랜치를 보호한다.

## 주요 시나리오

1. **dry-run 으로 삭제 대상 확인**: 사용자가 `git-tidy` 를 실행하면 정리될 브랜치
   목록만 표시되고 실제 삭제는 일어나지 않는다. 사용자는 무엇이 지워질지 먼저
   확인한다.
2. **실제 정리 실행**: 사용자가 `git-tidy --run` 을 실행하면 보호 규칙을 적용한 뒤
   정리 대상 브랜치를 실제로 삭제한다.

## 수용 기준

- 인자 없는 `git-tidy` 호출은 dry-run 으로 동작해 삭제 대상만 표시하고 브랜치를
  건드리지 않는다. 실제 삭제는 `--run` 을 명시할 때만 일어난다.
- git-tidy 는 보호 규칙으로 삭제하면 안 되는 브랜치를 정의하고, 보호되지 않은
  브랜치를 정리 대상으로 삼는다. 단일 upstream 상태 판정 한 가지에만 의존하지
  않는다.
- 같은 저장소 상태에서 git-tidy 를 여러 번 실행해도 추가 부작용 없이 같은 결과가
  나온다.

## 외부 의존

- **git**: 브랜치 조회, `git fetch --prune`, 브랜치 삭제에 사용한다. 필수 의존이다.
- **zsh**: git-tidy 자체가 zsh 플러그인이다. 필수 의존이다.
- **원격 저장소**: `git fetch --prune` 단계에 필요하다. `--no-fetch` 로 이 단계를
  건너뛸 수 있으므로 선택적 의존이다.

## 품질 dimension 선언

본 도구는 `docs/plans/2026-05-21-tool-quality-framework.md` 의 18개 dimension 에 대해
다음과 같이 선언한다. opt-out 인 항목은 reason 한 줄 필수.

| dimension | 상태 | reason (opt-out 시) |
|---|---|---|
| version_line | opt-in | |
| zsh_completion | opt-in | |
| bash_completion | opt-out | zsh 플러그인 전용 도구로 bash 에서 동작하지 않는다 |
| zsh_shell_integration | opt-in | |
| bash_shell_integration | opt-out | zsh 플러그인 전용 도구로 bash 에서 동작하지 않는다 |
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
| test_quality | opt-out | Go 도구가 아니라 coverage·golangci-lint 측정 대상이 아니다 |
| help_format_standard | opt-in | |
