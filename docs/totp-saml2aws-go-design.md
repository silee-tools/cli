# totp / saml2aws-auto Go 재작성 설계 라운드

작성일: 2026-05-09
상태: 결정 대기 (실행 전 사용자 검토)

## 0. 문서의 목적

기존 `silee9019/zsh-plugins` 안의 `totp.plugin.zsh` 와 `saml2aws-auto.plugin.zsh` 를 silee-tools/cli 모노레포의 `apps/totp/` 와 `apps/saml2aws-auto/` 안에 standalone Go CLI 로 재작성한다. 이 문서는 작성 시작 전에 결정해야 하는 설계 항목 다섯 가지를 정리하고, 각각에 대한 권고안과 트레이드오프를 제시해 사용자 확정을 받기 위한 자료다.

## 1. 기존 동작 요약 (재현 대상)

### 1.1 totp

macOS Keychain 의 generic-password 항목을 TOTP secret 저장소로 사용한다. 항목은 `service=<name>, account=$USER` 키로 식별되며, `desc` 필드에 `"TOTP (totp.plugin.zsh)"` 마커를 두어 totp 가 관리하는 항목만 골라낸다.

명령어 표면.

| 명령 | 동작 |
|---|---|
| `totp` | fzf picker 로 마커 항목 중 하나 선택 → 6자리 코드 출력 |
| `totp <name>` | 해당 항목의 시크릿으로 6자리 코드 stdout + 클립보드 복사 |
| `totp add <name>` | 시크릿을 Keychain 에 등록하고 마커 부착 |
| `totp rm <name>` | Keychain 에서 제거 |
| `totp ls [pattern]` | 마커 항목 나열 |
| `totp ls --all [pat]` | 마커 무시하고 모든 generic-password 나열 |
| `totp tag <name>` | 기존 항목에 마커 부착 (zsh 함수에서 마이그레이션) |

TOTP 계산은 base32 → HMAC-SHA1 → 30초 윈도우의 표준 RFC 6238.

### 1.2 saml2aws-auto-login

본인 회사(imagoworks) 의 AzureAD 기반 saml2aws 로그인을 자동화한다. AzureAD 가 SSO/cached cookies 로 비밀번호 단계를 건너뛰는 상태를 전제로, totp 가 생성한 6자리 코드를 `saml2aws login --mfa-token` 으로 주입한다.

기본 TOTP 항목 이름은 `${SAML2AWS_AUTO_TOTP_NAME:-MS: ${SAML2AWS_USERNAME}}` 로 환경변수에서 결정된다.

명령어 표면은 단일 `saml2aws-auto-login` 호출로 끝난다. 인자 없음.

## 2. 결정 항목

### 2.1 Keychain 접근 방식

| 옵션 | 장점 | 단점 |
|---|---|---|
| (a) `security` CLI 호출 (현행 zsh 구현과 동일) | 외부 의존 0, 동작 검증된 코드 경로 그대로 | os/exec 와 텍스트 파싱 의존, 마커 검색을 위해 `security dump-keychain` 출력 정규식 파싱 필요 |
| <u>**(b) `github.com/keybase/go-keychain` 라이브러리**</u> | Apple Security framework 직접 호출, 구조화된 결과, 타입 안전 | 외부 모듈 의존 추가, cgo 빌드 필요 (cross-compile 시 약간의 마찰) |

권고: **(b) go-keychain**. 마커 검색이 텍스트 파싱 대비 훨씬 안정적이며, 본인 macOS 사용자만 대상이라 cgo 가 문제되지 않는다. cross-compile 도 GitHub Actions 의 macOS runner 에서 처리.

### 2.2 TOTP 라이브러리

| 옵션 | 장점 | 단점 |
|---|---|---|
| (a) 표준 라이브러리만 (`crypto/hmac`, `crypto/sha1`, `encoding/base32`) | 외부 의존 0, 코드량 30 줄 미만 | 자체 작성 코드 검증 부담 (단, 표준 알고리즘이라 사실상 무위험) |
| <u>**(b) `github.com/pquerna/otp`**</u> | RFC 6238/4226 검증된 구현, otpauth URL 파싱·QR 코드 같은 부수 기능 무료 | 외부 모듈 의존 1개 |

권고: **(b) pquerna/otp**. 향후 totp add 가 otpauth URL(`otpauth://totp/...`) 입력을 받는 확장도 가능. 추가 비용은 모듈 1개.

### 2.3 모듈 구조 (go.mod 분리 정책)

| 옵션 | 장점 | 단점 |
|---|---|---|
| (a) 두 도구가 같은 go.mod 공유 (`apps/totp` 와 `apps/saml2aws-auto` 가 한 모듈) | 공통 코드 재사용 가능, 의존 관리 단일 | 모노레포 컨벤션(도구 사이 코드 공유 금지) 위반, 한 도구 변경이 다른 도구 빌드 트리거 |
| <u>**(b) 각자 독립 go.mod**</u> | 모노레포 컨벤션 준수, 도구 사이 의존 0 (saml2aws-auto-login 은 totp 바이너리를 PATH 룩업으로만 호출) | go-keychain wrapper 등 공통 코드 살짝 중복 가능 (saml2aws-auto-login 은 Keychain 안 봐도 됨) |

권고: **(b) 각자 독립 go.mod**. saml2aws-auto-login 은 Keychain 자체를 보지 않고 외부 `totp` 명령을 호출만 하므로 go-keychain 의존이 없다 — 자연스럽게 분리된다.

### 2.4 빌드 / 배포 방식

| 옵션 | 장점 | 단점 |
|---|---|---|
| (a) Source build (homebrew formula 가 `go build` 로 빌드) | release.yml 단순, artifact 업로드 불필요, 현 release.yml 그대로 | 사용자 머신에 Go toolchain 필요 (homebrew 가 `depends_on "go" => :build` 로 자동 처리) |
| <u>**(b) Source build (현 §5 결정)**</u> | 위와 동일, 현재 homebrew-tap formula 갱신과 일관 | 동일 |
| (c) Prebuilt 바이너리 업로드 (release.yml 이 GoReleaser 로 multi-arch 빌드 후 첨부) | 설치 빠름, 사용자 머신 toolchain 불필요 | release.yml 복잡도 증가, GoReleaser 설정 추가 |

권고: **(b) Source build**. Task #5 의 homebrew-tap formula 가 이미 모든 도구를 source-build 로 통일한 것과 일관. 본인 단독 사용이라 설치 속도 차이는 무시 가능.

### 2.5 환경변수 호환성

| 옵션 | 장점 | 단점 |
|---|---|---|
| <u>**(a) 기존 환경변수 이름 그대로 유지**</u> (`SAML2AWS_USERNAME`, `SAML2AWS_PASSWORD`, `SAML2AWS_SESSION_DURATION`, `SAML2AWS_AUTO_TOTP_NAME`) | `~/.config/zsh/saml2aws.zsh` 와 `.zshrc` 변경 최소화, .zshrc 마이그레이션 가이드대로 multisrc 한 줄만 빼면 됨 | 이름이 `SAML2AWS_*` 인데 saml2aws CLI 가 직접 인식하지 않는 것도 섞여 있어 약간 혼란 (현행 그대로) |
| (b) `SAML2AWS_AUTO_*` prefix 로 통일 | 도구 출처 명확 | 사용자 환경 추가 마이그레이션 필요 |

권고: **(a) 기존 이름 유지**. 본인 환경 변경을 최소화. 별칭이 필요해지면 나중에 추가.

## 3. 권고안 종합

| 항목 | 결정 (권고) |
|---|---|
| Keychain 접근 | go-keychain 라이브러리 |
| TOTP | pquerna/otp |
| 모듈 구조 | 각자 독립 go.mod |
| 빌드/배포 | Source build (homebrew formula 가 빌드) |
| 환경변수 | 기존 이름 그대로 유지 |

## 4. 명령 표면 호환

기존 zsh 함수의 명령 표면을 100% 동일하게 유지한다 (`totp <name>`, `totp ls`, `totp add` 등). 새 기능(otpauth URL 파싱 등) 추가는 보류 — 첫 릴리스는 1:1 동등 재현이 목표.

`saml2aws-auto-login` 도 인자 없는 단일 호출 형태 유지. 종료 코드와 stderr 메시지도 가급적 동일하게.

## 5. 작성 작업 순서 (결정 후)

이 문서의 권고안이 확정되면 다음 순서로 진행한다.

1. `apps/totp/` 신규 생성 — go.mod, cmd/totp/main.go, internal/keychain, internal/totp 골격
2. 기존 zsh 함수 동작을 표준 입출력 단위로 옮기는 단위 테스트 (Keychain mock + RFC 6238 표준 벡터)
3. `apps/saml2aws-auto/` 신규 생성 — go.mod, cmd/saml2aws-auto-login/main.go, exec.LookPath("totp") + os/exec
4. 두 도구 mise 태스크 (`fmt-check`, `lint`, `test`, `build`) 작성, 루트 README 도구 표 갱신
5. `.github/workflows/totp-ci.yml`, `.github/workflows/saml2aws-auto-ci.yml` 추가
6. homebrew-tap 에 `totp.rb`, `saml2aws-auto.rb` 신규 formula 추가
7. 첫 릴리스 (`totp/v0.1.0`, `saml2aws-auto/v0.1.0`) 후 `.zshrc` 마이그레이션 가이드 (docs/migration-zshrc.md) 따라 PATH 전환

## 6. 다음 세션 진입점

이 문서를 다시 열고 §3 권고안을 그대로 채택할지, 일부 항목을 (a)/(c) 로 바꿀지만 결정하면 작성 작업으로 곧장 진입할 수 있다.
