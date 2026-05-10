# jg

## Development

Run `mise tasks` to list available tasks.

## 릴리스

- Release PR merge → release-please 가 `jg/v<MAJOR>.<MINOR>.<PATCH>` 태그 생성 → GoReleaser → Homebrew tap 자동 업데이트
- `scripts/release.sh`는 이미 생성된 `jg/v*` 태그의 artifact를 재빌드할 때 `release-please.yml` workflow_dispatch를 실행한다. 새 버전 태그는 로컬에서 직접 만들지 않는다.
- `HOMEBREW_TAP_TOKEN` secret 필요 (homebrew-tap 레포 push 권한)
