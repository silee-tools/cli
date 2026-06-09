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

Homebrew로 설치한 뒤 셸 연동을 설정하려면 setup 명령을 한 번 실행합니다:

```bash
jg setup
```

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
mkdir -p "${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}/plugins/jg"
ln -sf "$(brew --prefix)/share/jg/plugin/jg.plugin.zsh" \
  "${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}/plugins/jg/jg.plugin.zsh"
```

`~/.zshrc`의 plugins에 `jg` 추가:

```zsh
plugins=(... jg)
```

## 사용법

```bash
jg              # fzf로 인터랙티브 점프 (저장소 안에서 실행하면 그 저장소의 main worktree 가 최상단에 고정)
jg <query>      # 쿼리로 필터링하여 점프
jg -l           # 추적 중인 모든 레포 목록 (점수 포함)
jg clean        # 오래되었거나 유효하지 않은 항목 제거
jg --clean      # 오래되었거나 유효하지 않은 항목 제거 (기존 옵션)
jg scheduler install  # macOS launchd에 매일 정리 작업 등록
jg scheduler status   # 정리 스케줄러 상태 확인
jg scheduler remove   # 정리 스케줄러 제거
jg --remove .   # 현재 디렉토리를 추적에서 제거
```

셸 연동 설정 후, Git 저장소에 `cd`하면 자동으로 추적됩니다.

## 주요 기능

- **frecency 기반 정렬**: 방문 빈도와 최근성을 결합한 스코어링
- **자동 수집**: 셸 hook을 통해 Git 저장소 방문 시 자동으로 기록
- **fzf 미리보기**: 브랜치, 최근 커밋, dirty status를 미리보기로 제공
- **정리 기능**: 삭제된 경로, 디렉토리가 아닌 경로, Git 저장소가 아닌 경로, submodule 항목 정리
- **예약 정리 기능**: `jg scheduler install` 명령으로 macOS launchd 기반 매일 정리 작업 등록
- **멀티 셸 지원**: Zsh, Bash 모두 지원
- **main worktree 고정**: git 저장소 안에서 인자 없이 `jg` 를 실행하면 그 저장소의 main working tree 가 피커 최상단에 고정되어, linked worktree 나 하위 디렉토리에서 빠르게 돌아갈 수 있다

## 개발

```bash
mise run build      # 빌드
mise run test       # 테스트 실행
mise run fmt-check  # gofmt 포맷 검사 (CI와 동일)
mise run install    # ~/.local/bin/jg에 설치
```
