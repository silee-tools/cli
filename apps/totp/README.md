# totp

macOS Keychain 기반 TOTP(Time-based One-Time Password) 코드 생성기.
기존 zsh 함수 `totp.plugin.zsh` 를 standalone Go CLI 로 1:1 동등 재현한 도구다.

> **macOS 전용**: 시크릿 저장소로 macOS Keychain(Apple Security framework)을
> 직접 호출하기 때문에 Linux/Windows 빌드는 제공하지 않는다.

## 설치

```bash
brew install silee-tools/tap/totp
```

또는 로컬 개발 빌드:

```bash
cd apps/totp
mise run install        # ~/.local/bin/totp
```

로컬 개발 빌드를 설치하면 활성 채널이 `dev`로 전환된다. 이후 Homebrew로
설치·업그레이드·재설치하면 활성 채널이 `release`로 전환되어, PATH에서 로컬 개발
바이너리가 먼저 발견되더라도 Homebrew 릴리스 바이너리를 실행한다. 다시
`mise run install`을 실행하기 전까지 별도의 PATH 정리나 개발 바이너리 삭제가 필요 없다.

## 사용

```bash
totp                              # fzf picker (마커 부착 항목만) → 코드 출력 + 클립보드 복사
totp "MS: you@example.com"        # 해당 항목으로 6자리 코드 출력 + 클립보드 복사
totp add "MS: you@example.com"    # 시크릿 등록 (입력 숨김) + 마커 부착
totp rm  "MS: you@example.com"    # 항목 제거 (alias: remove, delete)
totp ls                           # 마커 항목 나열
totp ls --all "MS:"               # 마커 무시하고 모든 generic-password 중 패턴 매칭
totp tag "MS: you@example.com"    # 기존 keychain 항목에 마커 부착 (마이그레이션용)
totp -h | --help                  # 도움말
totp -v | --version               # 버전
```

## 저장 컨벤션

- 위치: macOS Keychain, 항목 종류 `generic-password`
- `service` = 사용자가 지정한 `<name>`
- `account` = 현재 `$USER`
- `Description` = `"TOTP (totp.plugin.zsh)"` — 본 도구가 관리하는 항목임을 식별하는 마커.
  기존 zsh 함수가 부착하던 마커와 **동일한 문자열을 그대로 유지**하므로,
  zsh 함수로 등록한 기존 항목은 별도 마이그레이션 없이 본 CLI 로 그대로 사용할 수 있다.

## 기존 zsh 함수와의 호환성

zsh 함수에서 등록한 항목은 마커 문자열이 동일하기 때문에 본 CLI 의 `totp` /
`totp ls` / `totp <name>` 으로 그대로 다룰 수 있다. 마커가 없는 기존 항목은
`totp tag <name>` 으로 마커를 부착하면 picker 와 `ls` 에 노출된다.

## 개발

```bash
mise run test    # 단위 테스트 (RFC 6238 표준 벡터 + Keychain mock 사이클)
mise run build   # ./totp 바이너리 빌드
mise run lint      # go vet
mise run fmt-check # gofmt 포맷 검사 (CI와 동일)
mise run fmt       # gofmt -w .
```

## 의존 라이브러리

- [`github.com/keybase/go-keychain`](https://github.com/keybase/go-keychain) — macOS Keychain 직접 호출 (cgo)
- [`github.com/pquerna/otp`](https://github.com/pquerna/otp) — RFC 6238 TOTP 구현
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) — `add` 시 시크릿 입력 숨김 처리
