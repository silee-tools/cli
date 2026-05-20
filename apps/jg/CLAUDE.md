# jg

## Development

Run `mise tasks` to list available tasks.

## 릴리스

- Release PR merge → release-please 가 `jg/v<MAJOR>.<MINOR>.<PATCH>` 태그 생성 → GoReleaser → Homebrew tap 자동 업데이트
- `scripts/release.sh`는 이미 생성된 `jg/v*` 태그의 artifact를 재빌드할 때 `release-please.yml` workflow_dispatch를 실행한다. 새 버전 태그는 로컬에서 직접 만들지 않는다.
- `HOMEBREW_TAP_TOKEN` secret 필요 (homebrew-tap 레포 push 권한)

## 셸 통합 변경 시 단일 소스 원칙

`jg init zsh` / `jg init bash` 가 emit 하는 함수 정의는 `plugin/jg.plugin.zsh`
와 `plugin/jg.plugin.bash` 가 단일 진실 소스다. `internal/shell/shell.go` 는
`plugin` 패키지의 embed 변수를 그대로 반환하는 얇은 래퍼이며 인라인 셸 코드를
갖지 않는다.

새 셸 함수·hook·자동완성 트리거를 추가할 때는:
1. `plugin/jg.plugin.zsh` 와 `plugin/jg.plugin.bash` 두 파일을 함께 갱신한다
   (양쪽 셸 모두에 일관되게 적용).
2. `internal/shell/shell_test.go` 의 required token 목록에도 새 함수 이름을
   추가해 한쪽 빠뜨림(drift) 을 단위 테스트가 잡아내도록 한다.
3. `shell.go` 자체는 건드리지 않는다.

이 구조는 zsh/bash plugin 파일이 자동으로 brew formula 의 `share/jg/plugin/`
경로에 ship 되며, `jg init zsh|bash` 도 동일 내용을 emit 한다는 두 가지 경로의
정합성을 동시에 보장한다.
