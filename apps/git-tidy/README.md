# git-tidy

upstream 이 사라진(`[gone]`) 로컬 git 브랜치를 안전하게 정리하는 순수 zsh 플러그인.
trunk 기반 + squash merge 워크플로우에서, 원격 PR 이 squash 머지되어 원격 브랜치가
삭제되면 로컬 추적 브랜치가 `[gone]` 상태로 남는다. git-tidy 는 이런 브랜치만
골라 보호 규칙을 적용한 뒤 정리한다.

## 설치

```bash
brew install silee-tools/tap/git-tidy
```

또는 이 저장소에서 로컬 설치:

```bash
cd apps/git-tidy
mise run install
```

`mise run install` 은 플러그인을
`${XDG_DATA_HOME:-$HOME/.local/share}/git-tidy/git-tidy.plugin.zsh` 로 복사한다.

## zsh 플러그인 로드

설치 후 `~/.zshrc` 에 다음 한 줄을 추가한다.

```zsh
[[ -f "${XDG_DATA_HOME:-$HOME/.local/share}/git-tidy/git-tidy.plugin.zsh" ]] && \
  source "${XDG_DATA_HOME:-$HOME/.local/share}/git-tidy/git-tidy.plugin.zsh"
```

Homebrew 로 설치한 경우 plugin 은
`$(brew --prefix)/share/git-tidy/git-tidy.plugin.zsh` 에 놓이며, 설치 직후
출력되는 caveats 안내의 경로를 그대로 source 하면 된다.

## 사용

```bash
git-tidy              # dry-run (삭제 대상만 표시, 기본 동작)
git-tidy --run        # 실제 삭제 실행
git-tidy --days=N     # 최근 N일 이내 커밋이 있는 브랜치 보호 (기본 7일)
git-tidy --no-fetch   # git fetch --prune 단계 건너뛰기
git-tidy --help       # 사용법 출력
```

별칭(alias):

- `gtidy` 는 `git-tidy` 와 같다.
- `gtidy!` 는 `git-tidy --run` 과 같다(실제 삭제).

## 동작 방식

기본 동작은 dry-run 이라, 명시적으로 `--run` 을 주기 전에는 어떤 브랜치도
삭제하지 않는다. 실행 순서는 다음과 같다.

1. `--no-fetch` 가 없으면 `git fetch --prune origin` 으로 원격 기준 추적 정보를
   먼저 갱신한다. 오프라인이거나 remote 가 없으면 경고만 남기고 계속 진행한다.
2. upstream 추적 상태가 `[gone]` 인 로컬 브랜치만 후보로 모은다.
3. 후보 중 다음은 삭제하지 않고 건너뛰거나 보호한다.
   - 현재 체크아웃된 브랜치, 기본 브랜치(main/master/trunk 등 자동 감지)
   - 다른 worktree 에서 체크아웃 중인 브랜치
   - 최근 보호 기간(기본 7일) 이내에 커밋이 있는 브랜치
4. 남은 브랜치를 삭제 대상으로 분류한다. dry-run 이면 목록만 출력하고,
   `--run` 이면 `git branch -D` 로 삭제한다.

기본 브랜치는 oh-my-zsh git 플러그인의 `git_main_branch()` 가 있으면 그것을
쓰고, 없으면 자체 폴백으로 `main`/`trunk`/`mainline`/`default`/`stable`/`master`
순으로 탐색한다.

## 환경 변수

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `GIT_TIDY_PROTECT_DAYS` | 최근 커밋 보호 기간(일). `--days=N` 으로 호출 시점에 덮어쓸 수 있다. | `7` |

## 개발

이 도구는 Go 코드가 없는 순수 zsh 플러그인이다. 검증은 zsh 문법 검사를 하드
게이트로 두고, `shellcheck` 는 보조 지표로 둔다.

```bash
mise run shell-check   # zsh -n + shellcheck -s bash
mise run install       # 로컬 XDG 경로에 설치
mise run uninstall     # 로컬 설치 제거
```
