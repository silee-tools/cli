# totp PRD

## 한 줄 정의

totp 는 macOS Keychain 을 시크릿 저장소로 사용하는 TOTP(시간 기반 일회용 비밀번호)
코드 생성기다. 시크릿을 Keychain 의 generic-password 항목으로 보관하고, 항목 이름을
입력하면 해당 항목의 6자리 코드를 생성한다.

## 대상 사용자

터미널에서 TOTP 를 사용하는 macOS 개발자다. 별도의 인증 앱을 열지 않고 터미널에서
바로 2단계 인증 코드를 얻고 싶은 사용자를 상정한다.

## 목표

- **시크릿을 OS Keychain 에 안전 보관**: TOTP 시크릿을 평문 파일이 아니라 macOS
  Keychain 의 generic-password 항목으로 저장한다.
- **터미널에서 빠르게 코드 획득**: 항목 이름을 직접 입력하거나 picker 로 골라 6자리
  코드를 출력하고, 동시에 클립보드에 복사한다.
- **자동완성 최대 제공**: subcommand 와 플래그 같은 정적 후보뿐 아니라 등록된 항목
  이름 같은 동적 후보까지, zsh 와 bash 양쪽 셸이 표현할 수 있는 범위까지 자동완성을
  제공한다.

## 비목표

- **시크릿 평문 출력·내보내기**: 저장된 TOTP 시크릿 자체를 평문으로 출력하거나
  파일로 내보내는 기능은 제공하지 않는다. totp 가 다루는 출력은 시크릿이 아니라
  그로부터 계산한 일회용 코드다.
- **HOTP 등 비-TOTP 방식**: 카운터 기반 HOTP 나 그 밖의 일회용 비밀번호 방식은
  지원하지 않는다. 시간 기반 TOTP 한 가지만 다룬다.

## 기능 범위

- `totp`: 마커가 부착된 항목을 fzf picker 로 골라 6자리 코드를 출력하고 클립보드에
  복사한다.
- `totp <name>`: 지정한 항목의 6자리 코드를 출력하고 클립보드에 복사한다.
- `totp add <name>`: 시크릿을 입력 숨김 상태로 받아 Keychain 에 등록하고 totp 마커를
  부착한다.
- `totp rm <name>`: 항목을 제거한다. alias 로 `remove`, `delete` 를 받는다.
- `totp ls [--all] [pattern]`: 마커가 부착된 항목을 나열한다. alias 로 `list` 를
  받으며, `--all` 은 마커를 무시하고 모든 generic-password 항목을 대상으로 한다.
- `totp tag <name>`: 마커가 없는 기존 Keychain 항목에 totp 마커를 부착한다.
- `totp help` / `totp -h` / `totp --help`: 도움말을 출력한다.
- `totp -v` / `totp --version`: 버전을 출력한다.

## 주요 시나리오

1. **항목 이름으로 바로 코드 획득**: 사용자가 `totp "MS: me@example.com"` 처럼 항목
   이름을 입력하면 6자리 코드가 출력되고 클립보드에 복사된다.
2. **picker 로 항목을 골라 코드 획득**: 사용자가 인자 없이 `totp` 를 입력하면 마커가
   부착된 항목들이 fzf picker 에 나오고, 고른 항목의 코드가 출력·복사된다.
3. **새 시크릿 등록**: 사용자가 `totp add "<name>"` 을 입력하면 시크릿을 입력 숨김
   상태로 받아 Keychain 에 저장하고 totp 마커를 부착한다.

## 수용 기준

- totp 가 RFC 6238 에 맞는 6자리 TOTP 코드를 출력하고, 동시에 그 코드를 클립보드에
  복사한다.
- `totp add` 로 등록한 시크릿이 macOS Keychain 의 generic-password 항목으로 저장된다.
- picker 와 `totp ls` 가 기본적으로 마커가 부착된 항목만 보여주며, `--all` 은 마커를
  무시하고 모든 generic-password 항목을 대상으로 한다.

## 외부 의존

- **macOS Keychain** (Apple Security framework): 시크릿 저장소다. totp 가 macOS
  전용인 근거다. 필수 의존이다.
- **클립보드** (`pbcopy`): 생성한 코드를 클립보드에 복사하는 데 사용한다. 필수
  의존이다.
- **fzf**: picker 모드에서만 사용한다. fzf 가 없어도 `totp <name>` 직접 호출은
  정상 동작하며, picker 모드만 사용할 수 없다. 선택적 의존이다.

## 향후 로드맵 후보

다음은 현재 기능 범위에 포함되지 않으며, 추후 도입을 검토하는 후보다.

- **QR 코드 등록**: QR 이미지를 읽어 시크릿을 등록하는 기능.
- **외부 인증 앱 동기화**: 외부 인증 앱과 항목을 주고받는 동기화 기능.

## 품질 dimension 선언

본 도구는 `docs/plans/2026-05-21-tool-quality-framework.md` 의 18개 dimension 에 대해
다음과 같이 선언한다. opt-out 인 항목은 reason 한 줄 필수.

| dimension | 상태 | reason (opt-out 시) |
|---|---|---|
| version_line | opt-in | |
| zsh_completion | opt-in | |
| bash_completion | opt-in | |
| zsh_shell_integration | opt-out | 직접 호출형 도구로 셸 함수 통합이 필요 없다 (점프 같은 셸 상태 변경 없음) |
| bash_shell_integration | opt-out | 직접 호출형 도구로 셸 함수 통합이 필요 없다 (점프 같은 셸 상태 변경 없음) |
| readme_install | opt-in | |
| readme_usage | opt-in | |
| unit_tests_exist | opt-in | |
| ci_workflow | opt-in | |
| goreleaser | opt-in | |
| completion_covers_help | opt-in | |
| readme_mentions_main_features | opt-in | |
| plugin_emits_main_commands | opt-out | totp 는 init subcommand 가 없어 plugin emit 대상이 아니다 |
| goreleaser_archive_completeness | opt-in | |
| formula_install_completeness | opt-in | |
| tests_execute_and_pass | opt-in | |
| test_quality | opt-in | |
| help_format_standard | opt-in | |
