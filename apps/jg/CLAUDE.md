# jg

## Development

Run `mise tasks` to list available tasks.

## 릴리스

- `main` push 또는 `workflow_dispatch` → GitHub Actions 검증 → 버전 태그 생성 → GoReleaser → Homebrew tap 자동 업데이트
- 수동 릴리스는 `scripts/release.sh`로 `release.yml` workflow를 dispatch한다. 로컬에서 직접 태그를 만들지 않는다.
- `HOMEBREW_TAP_TOKEN` secret 필요 (homebrew-tap 레포 push 권한)
