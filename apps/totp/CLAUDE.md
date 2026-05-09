# totp

macOS Keychain 기반 TOTP CLI. 기존 zsh 함수 `totp.plugin.zsh` 의 1:1 Go 포팅.

**macOS 전용**: cgo + Apple Security framework 의존(`github.com/keybase/go-keychain`)으로 darwin 만 지원. Linux 에서는 `Keychain` 구현이 stub 으로 빠지지만 빌드는 통과해 `go vet`/`go test ./internal/...` 만큼은 cross-platform 으로 돌아간다.

## Development

`mise tasks` 로 사용 가능한 작업 목록 확인. 자주 쓰는 것:

- `mise run test` — 단위 테스트 (RFC 6238 표준 벡터 + Mock Store 사이클)
- `mise run build` — `./cmd/totp` 빌드
- `mise run lint` — `go vet ./...`
- `mise run fmt-check` — gofmt diff 검사

## 구조

- `cmd/totp/main.go` — CLI 진입점, 표준 라이브러리 `os.Args` 기반 분기
- `internal/store/` — Keychain 추상화. `Store` 인터페이스 + `Keychain`(darwin) / `MockStore`
- `internal/code/` — `pquerna/otp` wrapper, RFC 6238 표준 벡터로 검증

## 릴리스

`totp/v<MAJOR>.<MINOR>.<PATCH>` 태그 prefix 로 GoReleaser 가 darwin/amd64 + darwin/arm64 바이너리 생성. CGO 필요 → 빌드 호스트는 macOS 러너여야 함.
