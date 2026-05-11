# .zshrc 마이그레이션 가이드 (saml2aws-auto)

이 문서는 기존 dotfiles 의 `~/.config/zsh/saml2aws.zsh` 자동 체크 로직을 `saml2aws-auto` 프로젝트가 배포하는 zsh plugin 으로 옮길 때의 적용 순서를 정리한다.

## 목표 상태

`saml2aws-auto`가 다음 두 가지를 함께 관리한다.

- `saml2aws-auto` Go 바이너리: `login`, `check`, `init zsh` 하위 명령 제공
- `saml2aws-auto.plugin.zsh`: zsh 시작 시 `saml2aws-auto check`만 호출하는 얇은 plugin

기존 `saml2aws-auto-login` 명령은 더 이상 제공하지 않는다.

## 설치

릴리스 전 로컬 검증에서는 다음 명령을 사용한다.

```bash
cd apps/saml2aws-auto
mise run install
```

설치 후 PATH 와 plugin 파일을 확인한다.

```bash
command -v saml2aws-auto
saml2aws-auto --version
test -f "${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh"
```

## zsh 설정

zinit 을 쓰는 경우 `~/.zshrc`에 다음 줄을 둔다.

```zsh
zinit snippet "${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh"
```

plugin manager 없이 직접 source 할 수도 있다.

```zsh
source "${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh"
```

설치 예시는 다음 명령으로도 확인할 수 있다.

```bash
saml2aws-auto init zsh
```

## 기존 설정 정리

기존 `~/.config/zsh/saml2aws.zsh`에 있던 세션 판정, suppress 파일 처리, 프롬프트 로직은 제거한다. 사용자별 username 은 새 파일을 만들지 않고 기존 `~/.saml2aws`에서 읽는다.

```zsh
source "${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh"
```

## 회귀 확인

```bash
zsh -n "${XDG_DATA_HOME:-$HOME/.local/share}/saml2aws-auto/saml2aws-auto.plugin.zsh"
saml2aws-auto check
saml2aws-auto login
```

마지막으로 새 터미널 탭을 열어 AWS 세션이 만료 또는 만료 임박 상태일 때 로그인 확인 프롬프트가 뜨는지 확인한다. 세션이 유효하면 아무 출력 없이 셸이 시작되는 것이 정상이다.
