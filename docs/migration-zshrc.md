# .zshrc 마이그레이션 가이드 (totp / saml2aws-auto)

이 문서는 기존 `silee9019/zsh-plugins` 의 zsh 함수로 살아 있던 `totp` 와 `saml2aws-auto-login` 을 silee-tools/cli 모노레포의 standalone Go CLI 로 전환할 때 본인의 `~/.zshrc` 와 부속 설정을 어떻게 바꾸는지 정리한 가이드다. Go 재작성이 완료되어 Homebrew 로 설치 가능해진 시점부터 적용한다.

## 현재 상태 (마이그레이션 전)

`~/.zshrc` 의 macOS zinit 셋업 블록에서 두 도구를 zsh-plugins 의 multisrc 로 로드하고 있다.

```zsh
# ~/.zshrc 안의 _zsh_setup_macos_zinit()
zinit ice pick"/dev/null" multisrc"claude-auth-mode/claude-auth-mode.plugin.zsh overmind/overmind.plugin.zsh totp/totp.plugin.zsh saml2aws-auto/saml2aws-auto.plugin.zsh"
zinit light silee9019/zsh-plugins
```

이 한 줄로 `totp ()`, `saml2aws-auto-login ()` 두 zsh 함수가 셸 환경에 로드된다. `~/.config/zsh/saml2aws.zsh` 가 그 함수를 wrapper 로 호출하는 자동 세션 체크 로직을 제공한다.

## 마이그레이션 후 목표 상태

두 도구가 PATH 상의 standalone 바이너리 (`/opt/homebrew/bin/totp`, `/opt/homebrew/bin/saml2aws-auto-login`) 가 되어, zinit 의 multisrc 목록에서 빠진다. `saml2aws.zsh` 의 함수 호출은 PATH 룩업으로 그대로 동작한다 (셸 함수 → 외부 명령으로 자연 전환).

## 단계별 적용 순서

### 1. Homebrew 로 두 도구 설치

```bash
brew install silee-tools/tap/totp
brew install silee-tools/tap/saml2aws-auto
```

설치 후 PATH 에 잡혔는지 확인.

```bash
command -v totp                # /opt/homebrew/bin/totp
command -v saml2aws-auto-login # /opt/homebrew/bin/saml2aws-auto-login
totp --version
saml2aws-auto-login --version
```

### 2. zsh 함수 정의 잔재 제거

zinit multisrc 가 아직 정의해 둔 셸 함수가 외부 바이너리보다 우선이므로, 새 셸 세션 전에 한 번 함수를 unset 한다.

```zsh
unfunction totp 2>/dev/null
unfunction saml2aws-auto-login 2>/dev/null
```

### 3. `~/.zshrc` 수정

`_zsh_setup_macos_zinit()` 함수 안의 multisrc 리스트에서 두 항목을 제거한다.

```zsh
# Before
zinit ice pick"/dev/null" multisrc"claude-auth-mode/claude-auth-mode.plugin.zsh overmind/overmind.plugin.zsh totp/totp.plugin.zsh saml2aws-auto/saml2aws-auto.plugin.zsh"
zinit light silee9019/zsh-plugins

# After
zinit ice pick"/dev/null" multisrc"claude-auth-mode/claude-auth-mode.plugin.zsh overmind/overmind.plugin.zsh"
zinit light silee9019/zsh-plugins
```

`~/.config/zsh/saml2aws.zsh` 는 변경 없이 둔다. 이 파일은 `saml2aws-auto-login` 명령을 호출하는데, zsh 함수가 사라진 뒤에는 PATH 의 동일 이름 바이너리가 그 자리를 메운다.

### 4. 새 셸 세션에서 회귀 테스트

```bash
exec $SHELL -l    # 또는 새 터미널 탭 열기
totp <secret-name>             # 기존과 동일하게 6자리 출력 확인
saml2aws-auto-login            # 평소 saml2aws 로그인 흐름 확인
```

이 시점에 한 번 회사 AWS 콘솔/CLI 접근까지 정상 동작하는 것을 눈으로 확인한다. 여기까지 통과해야 다음 단계인 zsh-plugins 의 totp/saml2aws-auto 디렉토리 제거가 안전하다.

### 5. zsh-plugins 의존 잔재 정리 (선택, 분리 commit)

회귀 테스트 통과 후 본인의 `silee9019/zsh-plugins` 레포에서 두 디렉토리를 제거한다 (별도 PR/commit). zsh-plugins 의 README plugin 표에서도 두 항목을 빼준다. 다른 프로젝트에서 import 하는 경로가 있는지는 grep 으로 한 번 더 확인한다.

```bash
cd ~/ResilioSync/silee-drive/Repositories/silee9019/zsh-plugins
git rm -r totp saml2aws-auto
# README.md 의 plugin 표에서 totp, saml2aws-auto 행 제거
git commit -m "chore: drop totp/saml2aws-auto (이주처: silee-tools/cli)"
```

이 단계는 Task #8 (구 레포 archive) 와 묶어서 진행하는 것이 자연스럽다.

## 위험과 대비

가장 큰 위험은 마이그레이션 도중 saml2aws-auto-login 실행 흐름이 깨져 회사 AWS 접근이 막히는 것이다. 다음 두 가지로 완화한다.

첫째, 단계 1 의 `brew install` 이 실패하거나 단계 4 의 회귀 테스트가 실패하면 단계 3 의 `~/.zshrc` 수정을 되돌리고 zsh 함수로 일단 복귀한다. 두 곳에 같은 이름의 도구가 동시에 존재해도 셸 함수 vs PATH 바이너리 우선순위는 함수가 이기므로 충돌은 일어나지 않는다.

둘째, `SAML2AWS_AUTO_TOTP_NAME` 등 saml2aws-auto-login 이 의존하는 환경변수 이름이 Go 재작성 과정에서 바뀌었는지 한 번 더 확인한다. 가급적 기존 환경변수 이름을 그대로 유지해 사용자 환경 변경을 최소화한다 — Go 재작성 설계 라운드(Task #7) 의 결정 항목으로 다룬다.
