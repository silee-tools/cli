# cli

`silee-tools` 조직 아래 개인용 CLI 도구들을 모은 모노레포. 각 도구는 `apps/<tool>/` 디렉토리에 자기 완결적으로 존재하며, 자체 `.mise.toml`/README/테스트를 갖는다.

## 저장소 원칙 (모든 설계 결정에 우선 적용)

- **범용성**: 사용자 머신의 toolchain 의존을 줄이고 다양한 환경에서 동일 동작. prebuilt 바이너리 배포가 source build 보다 우선.
- **멱등성**: 같은 입력에 대해 몇 번 실행해도 같은 결과. 설치/업그레이드/마이그레이션 절차가 중간 상태에 의존하지 않도록.
- **안정성**: 텍스트 파싱·외부 명령 형식 의존을 줄이고 라이브러리 호출 등 구조화된 인터페이스를 선택. 하위 호환성 가급적 유지.

옵션 비교 시 이 세 원칙을 가산점 기준으로 사용한다.

## 개발

- Task Runner: mise (루트 `.mise.toml` = 공통 dev 도구, `apps/<tool>/.mise.toml` = 도구별 런타임/태스크)
- 도구별 작업 시 해당 디렉토리로 이동 후 mise task 실행:
  - `cd apps/<tool> && mise run test`
  - `cd apps/<tool> && mise run build`

## 모노레포 컨벤션

- 한 도구 = `apps/<tool>/` 한 디렉토리. 도구 사이 코드 공유 금지 (각자 독립).
- 현재 등록된 도구: jg, totp, saml2aws-auto, git-tidy. saml2aws-auto 는 `login`/`check`/`init zsh` 하위 명령과 zsh plugin 을 함께 제공한다. git-tidy 는 Go 바이너리가 없는 순수 zsh 플러그인 도구로, `.goreleaser.yaml` 이 `builds: [{skip: true}]` + `archives` 의 `meta: true` 로 빌드 없이 아카이브만 만들어 공통 릴리스 파이프라인을 통과한다.
- 도구 추가: `apps/<new-tool>/` 생성, 그 안에 `.mise.toml` + `README.md` + 진입점 + `.goreleaser.yaml` 를 둔다. 공통 릴리스 workflow 는 모든 도구가 GoReleaser 기반이라고 가정하므로, GoReleaser 설정 없이 추가하지 않는다. 루트 `README.md` 의 도구 표도 갱신한다.
- 릴리스: 태그를 직접 만들지 않는다. main 의 Conventional Commits 가 누적되면 release-please-action 이 도구별 Release PR 을 자동 생성/갱신하므로, 그 PR 의 본문(다음 버전 + CHANGELOG 변경분) 을 review 하고 merge 하는 것이 곧 릴리스 결정이다. PR merge 시 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 태그와 빈 GitHub Release 가 자동 생성되고, 후속 matrix job 이 GoReleaser 로 artifact 를 빌드해 첨부한 뒤 homebrew-tap formula 의 sha256/version 을 자동 commit + push 한다.
- CI: 도구별 `.github/workflows/<tool>-ci.yml` + paths 필터로 자기 디렉토리만 트리거.
- Homebrew formula 는 별도 레포 `silee-tools/homebrew-tap` 에서 관리. 본 레포의 source URL/buildpath 만 갱신 대상.

## 새 도구 추가 체크리스트

1. `apps/<tool>/` 생성, 도구 코드 + `.mise.toml` + README + `.goreleaser.yaml` 추가
2. `.github/workflows/<tool>-ci.yml` 추가 (paths 필터: `apps/<tool>/**`)
3. 루트 `README.md` / `docs/README_ko.md` 의 도구 표 갱신
4. `release-please-config.json` 의 `packages` 에 `"apps/<new-tool>": {"package-name": "<new-tool>"}` 추가
5. `.release-please-manifest.json` 에 `"apps/<new-tool>": "0.0.0"` 추가 (첫 `feat` commit 이 0.1.0 으로 자동 bump)
6. homebrew-tap 에 `Formula/<new-tool>.rb` 골격 작성 (sha256 placeholder). 첫 릴리스 후 release-please.yml 의 후속 step 이 sha256/version 자동 갱신.
