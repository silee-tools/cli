# jg

[English (영어)](../README.md)

Frecency 기반 Git 저장소 빠른 점프 CLI

자주 방문하는 Git 저장소를 frecency(빈도 + 최근성) 알고리즘으로 순위를 매기고, fzf를 통해 빠르게 선택하여 이동할 수 있는 도구입니다.

## 설치

```bash
brew install silee-tools/tap/jg
```

`fzf`가 의존성으로 자동 설치됩니다.

## 셸 설정

Homebrew로 설치하면 셸 연동이 자동으로 설정됩니다.

### 수동 설정

**방법 1: eval**

`~/.zshrc`에 추가:

```zsh
eval "$(jg init zsh)"
```

또는 Bash의 경우 `~/.bashrc`에 추가:

```bash
eval "$(jg init bash)"
```

**방법 2: oh-my-zsh 플러그인** (oh-my-zsh 사용자 권장)

```zsh
ln -sf $(brew --prefix)/share/jg/plugin/jg.plugin.zsh \
  ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/jg/jg.plugin.zsh
```

`~/.zshrc`의 plugins에 `jg` 추가:

```zsh
plugins=(... jg)
```

## 사용법

```bash
jg              # fzf로 인터랙티브 점프
jg <query>      # 쿼리로 필터링하여 점프
jg -l           # 추적 중인 모든 레포 목록 (점수 포함)
jg --clean      # 존재하지 않는 디렉토리 항목 제거
jg --remove .   # 현재 디렉토리를 추적에서 제거
```

셸 연동 설정 후, Git 저장소에 `cd`하면 자동으로 추적됩니다.

## 주요 기능

- **frecency 기반 정렬**: 방문 빈도와 최근성을 결합한 스코어링
- **자동 수집**: 셸 hook을 통해 Git 저장소 방문 시 자동으로 기록
- **fzf 미리보기**: 브랜치, 최근 커밋, dirty status를 미리보기로 제공
- **정리 기능**: 삭제된 디렉토리 entry 자동 정리
- **멀티 셸 지원**: Zsh, Bash 모두 지원

## 개발

```bash
mise run build      # 빌드
mise run test       # 테스트 실행
mise run install    # ~/.local/bin/jg에 설치
```
