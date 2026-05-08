# cli

`silee-tools` 조직 아래 개인용 CLI 도구들을 모은 모노레포. 각 도구는 `apps/<tool>/` 디렉토리에 자기 완결적으로 존재하며, 자체 `.mise.toml`/README/테스트를 갖는다.

## 개발

- Task Runner: mise (루트 `.mise.toml` = 공통 dev 도구, `apps/<tool>/.mise.toml` = 도구별 런타임/태스크)
- 도구별 작업 시 해당 디렉토리로 이동 후 mise task 실행:
  - `cd apps/<tool> && mise run test`
  - `cd apps/<tool> && mise run build`

## 모노레포 컨벤션

- 한 도구 = `apps/<tool>/` 한 디렉토리. 도구 사이 코드 공유 금지 (각자 독립).
- 도구 추가: `apps/<new-tool>/` 생성, 그 안에 `.mise.toml` + `README.md` + 진입점. 루트 `README.md` 의 도구 표 갱신.
- 릴리스 태그: `<tool>/v<MAJOR>.<MINOR>.<PATCH>` prefix 스킴. 모든 도구는 모노레포에서 v0.1.0 부터 시작.
- CI: 도구별 `.github/workflows/<tool>-ci.yml` + paths 필터로 자기 디렉토리만 트리거.
- Homebrew formula 는 별도 레포 `silee-tools/homebrew-tap` 에서 관리. 본 레포의 source URL/buildpath 만 갱신 대상.

## 새 도구 추가 체크리스트

1. `apps/<tool>/` 생성, 도구 코드 + `.mise.toml` + README 추가
2. `.github/workflows/<tool>-ci.yml` 추가 (paths 필터: `apps/<tool>/**`)
3. 루트 `README.md` / `docs/README_ko.md` 의 도구 표 갱신
4. 첫 릴리스 시 `<tool>/v0.1.0` 태그 push
