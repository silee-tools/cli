# saml2aws-auto

`saml2aws` AzureAD 로그인의 TOTP MFA 코드를 자동 주입하는 표준 라이브러리 전용 Go CLI. 기존 zsh 함수(`saml2aws-auto-login`)의 1:1 동등 재현.

전제: AzureAD 가 SSO/cached cookies 로 비밀번호 단계를 건너뛰는 상태. `--password=""` 와 `--skip-prompt` 로 비밀번호 입력을 우회하고, PATH 의 `totp` 바이너리에서 받은 6자리 코드를 `--mfa-token=` 으로 주입한다.

## 설치

```bash
brew install silee-tools/tap/saml2aws-auto
```

또는 로컬 빌드:

```bash
cd apps/saml2aws-auto
mise run install   # ~/.local/bin/saml2aws-auto-login
```

## 의존

런타임에 PATH 에 다음 두 바이너리가 있어야 한다.

- [`saml2aws`](https://github.com/Versent/saml2aws) — AWS SSO 로그인 본체
- [`totp`](https://github.com/silee-tools/cli/tree/main/apps/totp) — 6자리 TOTP 코드 생성기 (모노레포 자매 도구지만 코드 결합 없음)

빌드 자체는 외부 Go 의존성이 없다(표준 라이브러리만).

## 환경변수

| 변수 | 출처 | 동작 |
|---|---|---|
| `SAML2AWS_USERNAME` | saml2aws CLI 표준 | TOTP 항목 이름 기본값 `MS: $SAML2AWS_USERNAME` 산출에 사용. saml2aws 본체도 직접 읽음. |
| `SAML2AWS_AUTO_TOTP_NAME` | 본 도구 전용 | 빈 값이 아니면 그대로 TOTP 항목 이름으로 사용 (`SAML2AWS_USERNAME` 폴백보다 우선). |
| `SAML2AWS_PASSWORD`, `SAML2AWS_SESSION_DURATION` | saml2aws CLI 표준 | 본 도구는 읽지 않음. 필요 시 saml2aws 본체가 직접 읽는다. |

## 사용

```bash
export SAML2AWS_USERNAME='alice@example.com'
saml2aws-auto-login
```

기본적으로 `totp "MS: alice@example.com"` 을 호출해 코드를 받고, `saml2aws login --force --skip-prompt --password="" --mfa-token=<코드>` 를 실행한다.

TOTP 항목 이름이 다르면:

```bash
export SAML2AWS_AUTO_TOTP_NAME='AWS Prod (alice)'
saml2aws-auto-login
```

## 종료 코드

| 코드 | 상황 |
|---|---|
| 0 | 성공 |
| 1 | 환경변수 누락 / TOTP 코드 획득 실패 / saml2aws 호출 자체 실패 |
| 127 | `saml2aws` 또는 `totp` 가 PATH 에 없음 |
| 그 외 | `saml2aws login` 의 종료 코드를 그대로 전파 |

## 옵션

```
saml2aws-auto-login [-h|--help] [-v|--version]
```

## 개발

```bash
cd apps/saml2aws-auto
mise run test
mise run build
```
