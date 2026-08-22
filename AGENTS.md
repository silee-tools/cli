<!-- CLAUDE.md is a symbolic link to AGENTS.md. -->
# cli

`silee-tools` 조직 아래 개인용 CLI 도구들을 모은 모노레포. 각 도구는 `apps/<tool>/` 디렉토리에 자기 완결적으로 존재하며, 자체 `.mise.toml`/README/테스트를 갖는다.

## 저장소 원칙 (모든 설계 결정에 우선 적용)

- **범용성**: 사용자 머신의 toolchain 의존을 줄이고 다양한 환경에서 동일 동작. prebuilt 바이너리 배포가 source build 보다 우선.
- **멱등성**: 같은 입력에 대해 몇 번 실행해도 같은 결과. 설치/업그레이드/마이그레이션 절차가 중간 상태에 의존하지 않도록.
- **안정성**: 텍스트 파싱·외부 명령 형식 의존을 줄이고 라이브러리 호출 등 구조화된 인터페이스를 선택. 하위 호환성 가급적 유지.
- **자동완성 최대 제공**: 각 도구는 사용자 셸 환경에서 가능한 모든 입력 지점에 자동완성을 제공한다. 정적 후보(subcommand, 플래그) 는 물론 동적 후보(파일/디렉토리/도구 자체가 보유한 항목명) 도 셸이 표현할 수 있는 한 함께 보완한다.

옵션 비교 시 이 네 원칙을 가산점 기준으로 사용한다.

## 개발

- Task Runner: mise (루트 `.mise.toml` = 공통 dev 도구, `apps/<tool>/.mise.toml` = 도구별 런타임/태스크)
- 도구별 작업 시 해당 디렉토리로 이동 후 mise task 실행:
  - `cd apps/<tool> && mise run test`
  - `cd apps/<tool> && mise run build`

## 모노레포 컨벤션

- 한 도구 = `apps/<tool>/` 한 디렉토리. 도구 사이 코드 공유 금지 (각자 독립).
- 현재 등록된 도구: git-tidy, git-update-default, jg/jgw, totp. 모든 등록 도구는 `apps/<tool>/` 아래에 자체 Go 진입점과 `internal/` 패키지, README, `.mise.toml`, GoReleaser 설정을 둔다.
- 도구 추가: `apps/<new-tool>/` 생성, 그 안에 `.mise.toml` + `README.md` + 진입점 + `.goreleaser.yaml` 를 둔다. 공통 릴리스 workflow 는 모든 도구가 GoReleaser 기반이라고 가정하므로, GoReleaser 설정 없이 추가하지 않는다. 루트 `README.md` 의 도구 표도 갱신한다.
- 릴리스: 태그를 직접 만들지 않는다. main 의 Conventional Commits 가 누적되면 release-please-action 이 **변경된 도구의 버전 bump 를 묶은 단일 Release PR** 을 자동 생성하거나 갱신한다(`release-please-config.json` 의 `separate-pull-requests: false`). 이렇게 하나의 PR 로 모으는 이유는, 도구별로 PR 을 분리하면 여러 PR 이 공유 파일 `.release-please-manifest.json` 을 동시에 수정해 반복적으로 충돌하기 때문이다. 그 PR 의 본문에서 변경된 도구의 다음 버전과 CHANGELOG 변경분을 검토하고 merge 하는 것이 릴리스 결정이다. PR merge 시 bump 된 도구마다 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 태그와 빈 GitHub Release 가 자동 생성되고, 후속 matrix job 이 해당 도구의 artifact 를 빌드해 첨부한 뒤 homebrew-tap formula 의 sha256/version 을 자동 commit + push 한다.
- CI: 도구별 `.github/workflows/<tool>-ci.yml` + paths 필터로 자기 디렉토리만 트리거.
- Homebrew formula 는 별도 레포 `silee-tools/homebrew-tap` 에서 관리. 본 레포의 source URL/buildpath 만 갱신 대상.

## 새 도구 추가 체크리스트

1. `apps/<tool>/` 생성, 도구 코드 + `.mise.toml` + README + `.goreleaser.yaml` + `completions/_<tool>` + `completions/<tool>.bash` 추가
2. `.github/workflows/<tool>-ci.yml` 추가 (paths 필터: `apps/<tool>/**`)
3. 루트 `README.md` / `docs/README_ko.md` 의 도구 표 갱신
4. `release-please-config.json` 의 `packages` 에 `"apps/<new-tool>": {"package-name": "<new-tool>"}` 추가
5. `.release-please-manifest.json` 에 `"apps/<new-tool>": "0.0.0"` 추가 (첫 `feat` commit 이 0.1.0 으로 자동 bump)
6. homebrew-tap 에 `Formula/<new-tool>.rb` 골격 작성 (sha256 placeholder). 첫 릴리스 후 release-please.yml 의 후속 step 이 sha256/version 자동 갱신.
7. 버전 플래그를 표준 형식으로 제공한다. 새 도구는 `-v` 와 `--version` 입력 시 표준 출력에 `<tool> v<version> © 2026 silee-tools` 한 줄을 찍고 종료 코드 0 으로 끝나야 한다. 이는 conformance 게이트(`scripts/check-version-format.sh` 와 `.github/workflows/version-conformance.yml`)가 `apps/*` 를 자동 순회하며 강제하므로 별도 등록은 필요 없다. 게이트의 OS 라우팅은 도구의 `.goreleaser.yaml` `goos` 를 단일 진실 소스로 삼는다(소문자 `linux` 가 있으면 ubuntu, `darwin` 전용이면 macos, `builds: [{skip: true}]` 인 순수 zsh 플러그인은 ubuntu 에서 source 후 검증). 따라서 `.goreleaser.yaml` 의 `goos` 를 실제 빌드 대상과 일치하게 유지한다. Go 도구는 `versionLine(name, version string) string` 같은 작은 순수 함수로 형식 한 줄을 만들어 표준 출력에 쓴다.
8. PATH 앞쪽의 개발 바이너리를 유지하면서 배포 버전을 선택할 수 있도록 런타임 채널 계약을 구현한다. `mise run install` 은 Homebrew prefix 아래 상태를 `channel=dev` 로 원자적으로 바꾸고, `internal/runtimechannel` 은 고정된 Homebrew 설치 경로만 선택하며, Formula `post_install` 은 같은 상태를 `channel=release` 로 바꾼다. `scripts/check-runtime-channel-contract.sh` 가 `apps/*` 를 자동 순회하므로 별도 등록은 필요 없다.
