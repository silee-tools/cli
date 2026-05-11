# saml2aws-auto

`saml2aws` AzureAD 로그인에 TOTP MFA 코드를 자동 주입하고, zsh 시작 시 AWS 세션 만료 여부를 확인하는 Go CLI다.

전제: AzureAD 가 SSO/cached cookies 로 비밀번호 단계를 건너뛰는 상태. `saml2aws-auto login`은 `--password=""`와 `--skip-prompt`로 비밀번호 입력을 우회하고, PATH의 `totp` 바이너리에서 받은 6자리 코드를 `--mfa-token=`으로 주입한다.

## 설치

```bash
brew install silee-tools/tap/saml2aws-auto
```

또는 로컬 빌드:

```bash
cd apps/saml2aws-auto
mise run install   # ~/.local/bin/saml2aws-auto
```

## 의존

런타임에 PATH에 다음 두 바이너리가 있어야 한다.

- [`saml2aws`](https://github.com/Versent/saml2aws) — AWS SSO 로그인 본체
- [`totp`](https://github.com/silee-tools/cli/tree/main/apps/totp) — 6자리 TOTP 코드 생성기

## 환경변수

| 변수 | 출처 | 동작 |
|---|---|---|
| `SAML2AWS_USERNAME` | saml2aws CLI 표준 | 빈 값이 아니면 TOTP 항목 이름 기본값 `MS: $SAML2AWS_USERNAME` 산출에 사용. 비어 있으면 `~/.saml2aws`의 `username`을 읽음. |
| `SAML2AWS_AUTO_TOTP_NAME` | 본 도구 전용 | 빈 값이 아니면 그대로 TOTP 항목 이름으로 사용 (`SAML2AWS_USERNAME` 폴백보다 우선). |
| `SAML2AWS_SESSION_DURATION` | saml2aws CLI 표준 | 로그인 시 `--session-duration`으로 명시 전달. 비어 있으면 12시간(`43200`)을 사용한다. |
| `SAML2AWS_PASSWORD` | saml2aws CLI 표준 | 본 도구는 비밀번호 입력을 우회하기 위해 `--password=""`를 전달한다. |

## 사용

즉시 로그인:

```bash
saml2aws-auto login
```

기본적으로 `~/.saml2aws`의 `username` 값을 읽어 `totp "MS: <username>"`을 호출하고, `saml2aws login --force --skip-prompt --session-duration=<초> --password="" --mfa-token=<코드>`를 실행한다. `SAML2AWS_USERNAME`이 이미 환경에 있으면 그 값을 우선한다. session duration은 `SAML2AWS_SESSION_DURATION` 값이 있으면 사용하고, 없으면 12시간(`43200`)을 사용한다.

TOTP 항목 이름이 다르면:

```bash
export SAML2AWS_AUTO_TOTP_NAME='AWS Prod (alice)'
saml2aws-auto login
```

zsh 시작 시 자동 체크:

```bash
saml2aws-auto check
```

이 명령은 `~/.aws/credentials`의 `x_security_token_expires`를 읽는다. 세션이 이미 만료됐거나 1시간 이내에 만료될 때만 로그인 여부를 묻는다. `오늘 그만 물어보기` 상태는 `${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-login-suppress`에 저장한다.

프롬프트용 상태만 출력:

```bash
saml2aws-auto status
```

출력은 `valid`, `expiring_soon:<분>`, `expired`, `unknown` 중 하나다. 프롬프트에서 AWS 상태를 보여주려면 이 명령을 직접 호출한다.

`SAML2AWS_USERNAME`처럼 사용자마다 다른 값은 zsh plugin이 설정하지 않는다. 기본값은 `~/.saml2aws`에서 읽고, 특별히 덮어써야 할 때만 환경변수를 사용한다.

## zsh plugin

`mise run install`로 로컬 설치하면 plugin 파일은 다음 경로에 놓인다.

```bash
${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh
```

`~/.zshrc`에는 다음 블록을 추가한다. 설치 위치를 하드코딩하지 않고, PATH에 잡힌 `saml2aws-auto` 명령 위치에서 plugin 경로를 계산한다.

```zsh
if (( $+commands[saml2aws-auto] )); then
  local saml2aws_auto_bin="${commands[saml2aws-auto]:A}"
  local saml2aws_auto_plugin="${saml2aws_auto_bin:h:h}/share/saml2aws-auto/saml2aws-auto.plugin.zsh"
  [[ -f "$saml2aws_auto_plugin" ]] && source "$saml2aws_auto_plugin"
  unset saml2aws_auto_bin saml2aws_auto_plugin
fi
```

설치 예시는 명령으로도 확인할 수 있다.

```bash
saml2aws-auto init zsh
```

Homebrew 릴리스 설치 후에는 같은 파일이 formula의 `share/saml2aws-auto/saml2aws-auto.plugin.zsh`에 설치된다.

## 종료 코드

| 코드 | 상황 |
|---|---|
| 0 | 성공, 세션 유효, 또는 프롬프트 표시가 필요 없음 |
| 1 | 환경변수 누락 / TOTP 코드 획득 실패 / saml2aws 호출 자체 실패 |
| 2 | 명령 사용법 오류 |
| 127 | `saml2aws-auto login` 실행 시 `saml2aws` 또는 `totp`가 PATH에 없음 |
| 그 외 | `saml2aws login`의 종료 코드를 그대로 전파 |

## 옵션

```text
saml2aws-auto <check|status|login|init zsh> [-h|--help] [-v|--version]
```

## 개발

```bash
cd apps/saml2aws-auto
mise run test
mise run shell-check
mise run build
```
