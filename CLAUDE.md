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
- totp / saml2aws-auto 는 기존 zsh 함수에서 standalone Go CLI 로 재작성한다. 두 도구 사이의 결합은 saml2aws-auto-login 이 PATH 에서 totp 바이너리 존재만 확인하는 한 줄 의존이 전부다.
- 도구 추가: `apps/<new-tool>/` 생성, 그 안에 `.mise.toml` + `README.md` + 진입점. 루트 `README.md` 의 도구 표 갱신.
- 릴리스 태그: `<tool>/v<MAJOR>.<MINOR>.<PATCH>` prefix 스킴. 기존 5개 도구(appback/beautiful-mermaid-cli/jg/mydesk/unid)는 구 레포의 마지막 태그에서 patch bump 한 버전을 첫 모노레포 릴리스로 잡는다 (예: 구 `appback v0.2.3` → 모노레포 `appback/v0.2.4`). totp / saml2aws-auto 는 신규 Go CLI 로 재작성하므로 `v0.1.0` 부터 시작.
- CI: 도구별 `.github/workflows/<tool>-ci.yml` + paths 필터로 자기 디렉토리만 트리거.
- Homebrew formula 는 별도 레포 `silee-tools/homebrew-tap` 에서 관리. 본 레포의 source URL/buildpath 만 갱신 대상.

## 새 도구 추가 체크리스트

1. `apps/<tool>/` 생성, 도구 코드 + `.mise.toml` + README 추가
2. `.github/workflows/<tool>-ci.yml` 추가 (paths 필터: `apps/<tool>/**`)
3. 루트 `README.md` / `docs/README_ko.md` 의 도구 표 갱신
4. 첫 릴리스 시 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 태그 push (기존 도구는 구 레포 마지막 태그에서 patch bump, 신규 도구는 v0.1.0)
