# saml2aws-auto

표준 라이브러리만 사용하는 Go CLI. `saml2aws-auto-login` 바이너리 하나.

## Development

`mise tasks` 로 사용 가능한 태스크 확인. 의존: PATH 의 `totp` 바이너리(런타임), `saml2aws`(런타임). 빌드/테스트는 의존 불필요.

## 릴리스

`saml2aws-auto/v<MAJOR>.<MINOR>.<PATCH>` 태그. 루트 `release-please.yml` workflow + GoReleaser → homebrew-tap 자동 갱신.
