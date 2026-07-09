# git-update-default 현재 브랜치 모드 설계

작성일: 2026-07-09

## 한 줄 정의

`git-update-default` 에 `--current` 플래그를 추가해, default branch 로 전환하는 대신 지금 체크아웃돼 있는 브랜치를 그 브랜치의 upstream 까지 fast-forward 만 하는 모드를 제공한다.

## 배경 문제

기존 `git-update-default` 는 항상 저장소의 default branch(main·master 등)로 전환한 뒤 그 브랜치를 원격 최신까지 맞춘다. 그래서 지금 작업 중인 feature 브랜치를 그 자리에서 원격 최신으로 당겨오고 싶을 때는 쓸 수 없다. 사용자는 브랜치를 바꾸지 않은 채 "지금 이 브랜치를 원격 최신으로 갱신"하는 동작을 원한다.

## 목표

- 브랜치 전환 없이, 현재 체크아웃된 브랜치를 그 브랜치의 upstream(`@{upstream}`)까지 fast-forward 한다.
- 기존 default 모드의 동작은 한 글자도 바꾸지 않는다(하위호환).
- fetch·dirty 처리·fast-forward·divergence 경고 같은 공유 로직을 재사용한다.

## 비목표

- 별도 도구(새 `apps/<tool>` 디렉터리)를 만들지 않는다. 로직의 90% 가 기존 도구와 겹치므로 CI·GoReleaser·release-please·homebrew formula 를 새로 배선하는 비용을 지지 않는다.
- 서브커맨드 구조로 CLI 형태를 바꾸지 않는다. 기존 무인자 호출의 하위호환을 유지하기 위해 플래그로만 노출한다.
- upstream 이 없을 때 origin/<같은이름> 을 추측해 당기지 않는다(의도치 않은 브랜치를 당기는 사고를 막기 위해 명시적 에러로 멈춘다).
- 갈라진(diverged) 브랜치를 강제로 맞추지 않는다. 기존과 동일하게 경고만 하고 멈춘다.

## 동작 명세

### 모드 분기

플래그가 없으면 기존 default 모드로 동작한다: default branch(main → master → gh → origin/HEAD 순)를 정하고, 그 브랜치로 전환한 뒤 `origin/<default>` 까지 fast-forward 한다.

`--current` 가 있으면 현재 브랜치 모드로 동작한다. 아래 절차를 따른다.

### `--current` 절차

1. 현재 위치가 git 작업 트리인지(`IsRepo`), origin 원격이 있는지(`HasOriginRemote`) 확인한다. 어느 하나라도 실패하면 기존과 같은 메시지로 종료 코드 1 로 멈춘다.
2. 체크아웃된 브랜치 이름을 확인한다. detached HEAD 여서 브랜치 이름이 비어 있으면 "detached HEAD 상태라 현재 브랜치를 갱신할 수 없다"는 메시지와 함께 종료 코드 1 로 멈춘다.
3. 현재 브랜치의 upstream 을 해석한다(`git rev-parse --abbrev-ref @{upstream}` → 예: `origin/feature-x`). upstream 이 설정돼 있지 않으면 "이 브랜치에 upstream 이 설정되지 않았다"는 메시지와 함께 종료 코드 1 로 멈춘다. upstream 을 추측하지 않는다.
4. `git fetch origin --prune` 로 원격 최신을 받는다. 실패(오프라인 등)는 기존과 같이 치명적이지 않게 처리해, 경고만 남기고 로컬의 원격 추적 참조로 계속한다.
5. 커밋되지 않은 변경이 있으면 기존 dirty 처리 로직을 그대로 재사용한다. `--stash`/`--force` 플래그가 있으면 묻지 않고 그 경로로, 플래그가 없으면 TTY 일 때 인터랙티브로 묻고 비-TTY 면 안전하게 취소로 멈춘다. `--current` 는 `--stash`/`--force` 와 조합할 수 있다.
6. 현재 브랜치를 3단계에서 구한 upstream ref 까지 `git merge --ff-only <upstream>` 으로 fast-forward 한다. 갈라져서 fast-forward 가 불가능하면 기존과 같은 형식의 경고를 내고 종료 코드 1 로 멈춘다(강제하지 않는다).
7. 성공하면 어떤 브랜치를 어느 upstream 최신까지 맞췄는지 한 줄로 알린다.

현재 브랜치 모드에서는 브랜치 전환(`switchTo`)을 호출하지 않는다.

## 코드 변경 범위

### `cmd/git-update-default/main.go`

- `options` 구조체에 `current bool` 필드를 더한다.
- `parseArgs` 에 `--current` 케이스를 더한다.
- `helpText` 에 `--current` 설명을 더한다.
- `run()` 에 모드 분기를 넣는다. default 경로는 기존 코드를 그대로 두고, current 경로를 별도 함수(예: `runCurrent`)로 분리해 upstream 해석·fetch·dirty 처리·fast-forward 를 수행한다. dirty 처리는 기존 `handleDirty` 를 공유한다.

### `internal/gitx/gitx.go`

- `Upstream() (string, error)` 를 더한다. `git rev-parse --abbrev-ref @{upstream}` 의 출력을 다듬어 돌려주고, upstream 이 없으면 에러를 돌려준다.
- `MergeFFOnlyRef(ref string) error` 를 더해 임의의 ref 로 fast-forward 하게 하고, 기존 `MergeFFOnly(name)` 가 `MergeFFOnlyRef("origin/"+name)` 을 호출하도록 리팩터한다(기존 동작 그대로 유지).

### 완성(completion) 파일

- `completions/_git-update-default`(zsh)와 `completions/git-update-default.bash` 에 `--current` 후보를 더한다.

### 문서

- `README.md` 의 동작·사용 절에 `--current` 를 반영한다.

### 변경하지 않는 것

- `.github/workflows/`, `.goreleaser.yaml`, `release-please-config.json`, `.release-please-manifest.json`, homebrew-tap formula 는 건드리지 않는다. 기존 도구 내부 변경이라 릴리스 배선에 영향이 없다.

## 알려진 한계

fetch 는 origin 스코프다(기존과 동일). 현재 브랜치의 upstream 이 origin 이 아닌 다른 원격을 추적하면 fetch 가 그 원격을 받지 않아 최신이 아닐 수 있다. 이때는 로컬에 이미 있는 원격 추적 참조로 fast-forward 하며, 최신이 아닐 수 있음은 fetch 실패 경고와 같은 수준의 위험으로 남긴다. 이 도구는 origin 중심 저장소를 전제한다.

## 수용 기준

- `--current` 없이 실행하면 기존 default 모드 동작이 그대로 유지된다.
- upstream 이 설정된 브랜치에서 `--current` 를 실행하면 브랜치 전환 없이 그 브랜치가 upstream 최신까지 fast-forward 된다.
- upstream 이 없는 브랜치에서 `--current` 를 실행하면 origin/<이름> 을 당기지 않고 종료 코드 1 로 멈춘다.
- detached HEAD 에서 `--current` 를 실행하면 종료 코드 1 로 멈춘다.
- 현재 브랜치가 upstream 과 갈라진 상태에서 `--current` 를 실행하면 강제 병합 없이 경고 후 종료 코드 1 로 멈춘다.
- dirty 상태에서 `--current --stash` / `--current --force` 조합이 각각 stash·폐기 후 진행한다.

## 검증 방식

- 단위 테스트로 `parseArgs` 의 `--current` 파싱, dirty 경로 조합, mode 분기(current 시 `switchTo` 미호출, default 시 기존 유지)를 Red → Green 으로 확인한다. Red 는 빈 구현에서 실행 실패를 눈으로 확인한다.
- 실제 git 저장소를 만들어 1회성 E2E 스모크를 돌린다: (a) upstream 이 있는 브랜치에서 `--current` 가 fast-forward 로 성공하는 경우, (b) upstream 이 없는 브랜치에서 종료 코드 1 로 멈추는 경우. 각 실행의 종료 코드와 브랜치 tip 위치를 증거로 남긴다.
