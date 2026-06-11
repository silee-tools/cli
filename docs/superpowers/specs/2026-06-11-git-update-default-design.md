# git-update-default 설계

## 한 줄 정의

git 저장소 안의 어느 하위 경로에서 실행하더라도, 그 저장소를 원격 default branch 의 최신 상태로 전환하는 명령줄 도구.

## 대상 사용자

여러 git 저장소를 오가며 작업하는 개인 사용자. 다른 브랜치에서 작업하다가 "원격 기준 최신 default branch 로 돌아가 최신 변경을 받고 싶다" 는 상황을 한 번의 명령으로 처리하려는 사람.

## 목표

- 현재 디렉토리가 git 저장소의 작업 트리 안이면, 그 위치가 저장소 루트가 아니라 어느 하위 경로이더라도 해당 저장소를 대상으로 동작한다.
- default branch 를 `main` 으로 가정하지 않고, 원격에서 매번 동적으로 탐색한다. 그래서 `master`, `develop` 등 어떤 이름이 default 이더라도 올바르게 동작한다.
- 원격의 최신 상태를 받아 default branch 를 그 최신 지점으로 맞춘다.
- 작업 트리에 커밋되지 않은 변경(dirty change)이 있으면, 변경 파일 목록을 보여 주고 사용자가 어떻게 처리할지 직접 고르게 한다. 사용자가 모르는 사이에 변경을 잃지 않게 한다.

## 비목표

- 하위 디렉토리를 순회하며 여러 저장소를 한 번에 동기화하지 않는다. 한 번의 실행은 현재 위치가 속한 단일 저장소만 대상으로 한다.
- push, 즉 로컬 변경을 원격으로 올리는 동작은 하지 않는다. 이 도구는 원격 → 로컬 방향의 최신화만 담당한다.
- 로컬 default branch 가 원격과 갈라진(diverge) 경우에 원격 기준으로 강제 재설정하지 않는다. 그런 상황은 경고만 하고 사용자의 판단에 맡긴다.

## 기능 범위와 동작 흐름

도구를 인자 없이 실행하면 아래 순서로 동작한다.

1. **git 저장소 루트 탐색.** `git rev-parse --show-toplevel` 로 현재 위치가 속한 저장소의 루트를 찾는다. git 저장소가 아니면 그 사실을 알리는 에러 메시지를 출력하고 0 이 아닌 종료 코드로 끝낸다.
2. **origin 원격 확인.** `origin` 원격이 없으면 원격 default branch 라는 개념이 성립하지 않으므로, 그 사실을 알리는 에러 메시지를 출력하고 0 이 아닌 종료 코드로 끝낸다.
3. **원격 최신 상태 가져오기.** `git fetch origin --prune` 으로 원격의 최신 커밋과 삭제된 원격 브랜치 정리를 반영한다.
4. **default branch 탐색.** 다음 우선순위로 default branch 이름을 정한다.
   - (a) 원격에 `origin/main` 이 있으면 `main` 으로 정한다.
   - (b) 없고 원격에 `origin/master` 가 있으면 `master` 로 정한다.
   - (c) 둘 다 없으면 `gh repo view --json defaultBranchRef --jq .defaultBranchRef.name` 로 GitHub 의 default branch 를 조회한다. `gh` 가 설치되어 있고 인증되어 있으며 GitHub 저장소인 경우에만 값을 얻는다.
   - (d) `gh` 로도 정하지 못하면(미설치, 비-GitHub, 조회 실패) `git symbolic-ref --short refs/remotes/origin/HEAD` 로 원격 HEAD 참조를 읽어 폴백한다.
   - 위 단계들로도 default branch 를 정하지 못하면 그 사실을 알리는 에러 메시지를 출력하고 0 이 아닌 종료 코드로 끝낸다.
5. **dirty 검사와 분기.** `git status --porcelain` 으로 작업 트리에 커밋되지 않은 변경이 있는지 본다.
   - **clean 인 경우**: 곧장 전환과 최신화 단계로 넘어간다.
   - **dirty 인 경우**: 변경된 파일들을 트리 형태로 출력하고, 다음 세 갈래를 인터랙티브하게 묻는다.
     - **stash 후 진행**: `git stash push -u` 로 변경(추적되지 않는 파일 포함)을 보관한 뒤 전환과 최신화를 진행한다. 보관한 stash 는 자동으로 복원하지 않고 그대로 남겨 두며, "원래 작업하던 브랜치로 돌아가 `git stash pop` 으로 복원하라" 고 안내한다. 자동으로 복원하지 않는 이유는, 이 도구가 끝나면 작업 트리가 default branch 위에 있어서 원래 브랜치가 아닌 곳에 변경이 잘못 얹힐 수 있기 때문이다.
     - **force(작업 트리 변경 폐기)**: `git reset --hard HEAD` 로 추적되는 파일의 커밋되지 않은 변경을 버린 뒤 전환과 최신화를 진행한다. 추적되지 않는 새 파일(untracked)은 지우지 않고 보존한다 — `reset --hard` 는 untracked 를 건드리지 않으며, 이 도구도 파괴 범위를 추적되는 변경으로 최소화한다. 이 선택은 추적되는 변경을 되돌릴 수 없으므로, 선택 화면의 기본 커서는 취소에 둔다.
     - **취소(기본값)**: 아무것도 바꾸지 않고 종료 코드 0 으로 끝낸다.
6. **default branch 로 전환.** 이미 default branch 에 있으면 전환을 건너뛴다. 아니면 `git switch <default>` 로 전환하고, 로컬에 해당 브랜치가 없으면 `origin/<default>` 를 추적하는 브랜치로 새로 만들어 전환한다.
7. **최신화.** default branch 위에서 `git merge --ff-only origin/<default>` 로 원격 최신 지점까지 fast-forward 한다. fast-forward 가 불가능한 경우(로컬 default branch 가 원격과 갈라진 경우)에는 강제로 맞추지 않는다. 대신 갈라졌다는 경고를 출력하고 0 이 아닌 종료 코드로 끝낸다. 원격 기준 강제 재설정이 필요하면 사용자가 직접 처리하도록 남긴다.

## 비대화(non-TTY) 동작

표준 입력이 터미널이 아니어서 인터랙티브 선택 화면을 띄울 수 없는 환경(파이프, CI, 자동화 스크립트)에서 dirty 를 만나면, 기본값인 **취소** 로 안전하게 중단하고 그 사실을 안내한다.

자동화나 종료 흐름에서 미리 정한 처리를 적용하려는 경우를 위해 두 플래그를 제공한다.

- `--stash`: dirty 일 때 인터랙티브 선택 없이 stash 후 진행을 수행한다.
- `--force`: dirty 일 때 인터랙티브 선택 없이 작업 트리 변경을 폐기하고 진행한다.

두 플래그를 모두 지정하지 않은 비대화 환경에서 dirty 이면 중단한다. 이 플래그들은 비-TTY 환경에서의 자동화뿐 아니라, expect 로 PTY 를 할당해 수행하는 종단 간(E2E) 검증에도 사용한다.

## 인터랙티브 선택 화면

dirty 처리의 세 갈래 선택은 형제 도구 git-tidy 와 같은 bubbletea 기반 화살표 선택 UI 로 구현한다. 화면 위쪽에는 변경 파일 트리를, 아래쪽에는 세 선택지(stash 후 진행 / force / 취소)를 둔다. 되돌릴 수 없는 force 의 오선택을 줄이기 위해 초기 커서는 취소에 둔다.

## 수용 기준

- git 저장소의 하위 디렉토리(루트가 아닌 경로)에서 실행해도 그 저장소를 대상으로 동작한다.
- default branch 가 `main` 이 아닌 저장소(`master`, `develop` 등)에서도 올바른 브랜치로 전환한다.
- clean 상태에서 실행하면 default branch 로 전환하고 원격 최신까지 fast-forward 한다. 같은 상태에서 다시 실행해도 결과가 같다(멱등).
- dirty 상태에서 실행하면 변경 파일 트리가 출력되고, stash / force / 취소 세 갈래를 고를 수 있다. 취소가 기본값이다.
- stash 를 고르면 변경이 보관된 채 default branch 로 전환되고, 복원 안내가 출력된다. stash 는 자동으로 복원되지 않는다.
- force 를 고르면 추적되는 변경이 폐기되고(추적되지 않는 새 파일은 보존) default branch 최신으로 맞춰진다.
- git 저장소가 아닌 곳, origin 원격이 없는 저장소, 로컬 default branch 가 원격과 갈라진 경우 각각 명확한 에러 또는 경고와 0 이 아닌 종료 코드로 끝난다.
- `-v` 또는 `--version` 을 주면 `git-update-default v<version> © 2026 silee-tools` 한 줄을 표준 출력에 찍고 종료 코드 0 으로 끝난다.
- 비-TTY 환경에서 dirty 이면서 `--stash` 와 `--force` 가 모두 없으면 중단한다. `--stash` 또는 `--force` 를 주면 해당 처리를 인터랙티브 없이 수행한다.

## 표준 인터페이스와 의존성 정렬

- 버전 출력은 저장소의 버전 형식 적합성 게이트(`scripts/check-version-format.sh`)가 요구하는 `git-update-default v<version> © 2026 silee-tools` 형식을 따른다. 형식 한 줄은 `versionLine(name, version string) string` 같은 작은 순수 함수로 만든다.
- bubbletea 는 형제 도구 git-tidy 와 동일하게 `v1.3.4` 로 핀하고, 그것이 끌어오는 indirect 의존도 같은 baseline 으로 핀해, 모듈의 `go` directive 를 형제 도구들과 같은 `go 1.23` 으로 유지한다.

## 저장소 편입 산출물

저장소의 새 도구 추가 체크리스트를 그대로 따른다.

- `apps/git-update-default/` 아래에 진입점 `cmd/git-update-default/`, 내부 패키지 `internal/`, `.mise.toml`, `README.md`, `.goreleaser.yaml`, 자동완성 `completions/_git-update-default` 와 `completions/git-update-default.bash` 를 둔다.
- `.github/workflows/git-update-default-ci.yml` 을 추가하고 paths 필터를 `apps/git-update-default/**` 로 둔다.
- 루트 `README.md` 와 `docs/README_ko.md` 의 도구 표에 새 도구를 추가한다.
- `release-please-config.json` 의 packages 에 `"apps/git-update-default": {"package-name": "git-update-default"}` 를 더하고, `.release-please-manifest.json` 에 `"apps/git-update-default": "0.0.0"` 을 더한다.
- 별도 저장소 `silee-tools/homebrew-tap` 에 `Formula/git-update-default.rb` 골격(sha256 placeholder)을 둔다. 첫 릴리스 후 릴리스 워크플로우의 후속 단계가 sha256 과 version 을 자동으로 채운다.

## 기존 git-sync-main 정리

옛 `git-sync-main` 은 사라진 dotfiles 저장소(`~/Repositories/dotfiles/Mackup`)에 있던 스크립트였고, 현재는 깨진 심볼릭 링크와 git alias 만 잔존한다. 이 잔존물을 정리한다. 이 정리는 저장소 코드가 아니라 사용자 머신의 설정을 바꾸는 작업이므로, 도구 구현 PR 과는 분리된 별도 단계로 다룬다.

- `~/.local/bin/git-sync-main` 깨진 심볼릭 링크를 제거한다. 이 경로가 chezmoi 의 관리 대상인지 먼저 확인하고, 관리 대상이면 chezmoi 소스에서 제거한 뒤 apply 한다.
- `git config --global --unset alias.sync-main` 으로 git alias 를 제거한다.
- `~/.zshrc` 의 `alias gsm='git sync-main'` 을 제거하고, `qq` alias 를 새 도구 기준으로 갱신한다. `~/.zshrc` 는 chezmoi 관리 대상이므로 홈 타깃을 직접 편집하지 않고 chezmoi 소스를 편집한 뒤 apply 한다. 편집 전에 `chezmoi status` 가 clean 한지 확인하고, 이미 drift 가 있으면 어떻게 정리할지 먼저 사용자에게 묻는다.
- 새 alias 는 `gud='git update-default'` 로 두고, `qq` 는 `gud && gtidy! && logout` 으로 갱신한다.

## 결정과 근거

- **default branch 를 우선순위 fallback 으로 탐색한다.** `main` 고정 이름은 default 가 다른 저장소에서 틀린 대상을 가리킨다. 실무에서 가장 흔한 `main` 과 `master` 를 원격 브랜치 존재로 먼저 확인하고, 둘 다 없을 때만 `gh` 로 GitHub default 를 조회하며, 그것도 안 되면 원격 HEAD 참조로 폴백한다. `gh` 를 마지막 단계에 두어, gh 가 없는 머신이나 비-GitHub 저장소에서도 동작하도록 toolchain 의존을 최소화한다.
- **diverge 시 원격으로 강제 맞추지 않는다.** 강제 재설정은 로컬 커밋을 잃게 할 수 있다. 흔치 않은 상황을 자동으로 파괴적으로 처리하기보다, 경고로 알리고 사용자에게 맡기는 편이 안정성과 멱등성에 부합한다.
- **stash 후 자동 복원하지 않는다.** 도구가 끝나면 작업 트리는 default branch 위에 있다. 그 자리에서 stash 를 복원하면 원래 작업 브랜치가 아닌 곳에 변경이 얹혀 혼란을 부른다. 보관만 하고 복원은 사용자가 원래 브랜치로 돌아가 직접 하게 한다.
- **종료 인터셉트는 명시적 alias 호출(`qq`)로 한정한다.** ctrl-d 나 logout 을 종료 훅(zshexit 등)으로 가로채는 방식은 프롬프트(ZLE)가 종료된 시점이라 dirty 의 인터랙티브 선택이 불안정하다. 사용자가 직접 친 alias 는 인터랙티브가 안정적으로 동작하므로, 종료 흐름에 도구를 끼워 넣는 일은 `qq` alias 갱신으로만 처리한다.

## 외부 의존

- git 명령줄 도구(저장소 탐색, fetch, 브랜치 전환, merge, stash, reset).
- `origin` 원격과 그 원격의 브랜치(`origin/main`, `origin/master`)·default branch 참조.
- `gh`(GitHub CLI) — default branch 탐색의 마지막에서 두 번째 단계에서만 선택적으로 사용. 없거나 비-GitHub 저장소이면 `origin/HEAD` 폴백으로 넘어간다(필수 의존 아님).
- bubbletea(`v1.3.4`) 인터랙티브 선택 UI 라이브러리.
- 사용자 머신의 chezmoi(기존 정리 단계에서 `~/.zshrc` 와 `~/.local/bin` 의 관리에 사용).
