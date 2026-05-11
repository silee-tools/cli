# saml2aws-auto

Go CLI. 대표 바이너리는 `saml2aws-auto` 하나이며 `login`/`check`/`init zsh` 하위 명령을 제공한다.

## Development

`mise tasks` 로 사용 가능한 태스크 확인. 의존: PATH 의 `totp` 바이너리(런타임), `saml2aws`(런타임). zsh plugin 검증은 `mise run shell-check` 로 수행한다.

## 릴리스

`saml2aws-auto/v<MAJOR>.<MINOR>.<PATCH>` 태그. 루트 `release-please.yml` workflow + GoReleaser → homebrew-tap 자동 갱신.
