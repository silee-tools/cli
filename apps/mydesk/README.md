# mydesk

macOS config backup & sync tool (Mackup alternative)

## 설치

```bash
# Homebrew (권장)
brew install silee-tools/tap/mydesk

# 또는 Go
go install github.com/silee-tools/mydesk@latest
```

## 사용법

### 1. 설정 레포 초기화

```bash
mydesk init ~/my-dotfiles
cd ~/my-dotfiles && git init
```

### 2. 설정 파일 추가

네이티브 디렉토리에 파일을 배치하면 자동으로 심볼릭 링크 대상이 됩니다:

| 디렉토리 | 대상 | 동작 |
|-----------|------|------|
| `home/` | `~/` | 각 파일을 `~/`에 심볼릭 링크 |
| `config/` | `~/.config/` | 각 파일을 `~/.config/`에 심볼릭 링크 |
| `ssh/` | `~/.ssh/` | SSH 설정 심볼릭 링크 |
| `vscode/` | VS Code User dir | 설정 심볼릭 링크 + extensions 관리 |
| `brew/` | - | Brewfile sync/install |
| `macos/` | - | macOS defaults 스크립트 실행 |
| `omz/` | - | Oh-My-Zsh 설치 스크립트 실행 |

```bash
# 예시: .zshrc 백업
cp ~/.zshrc ~/my-dotfiles/home/.zshrc
```

### 3. 셸 프로파일 설정

```bash
mydesk --config-dir ~/my-dotfiles install-shell
source ~/.zprofile  # 또는 셸 재시작
```

이후 `--config-dir` 없이 사용 가능합니다.

### 4. 심볼릭 링크 생성

```bash
mydesk link
```

### 5. 추가 매핑 (links.conf)

네이티브 규약 외 경로는 `links.conf`에 선언:

```
# 외부 레포의 파일을 심볼릭 링크
$REPOS/my-org/some-repo/config -> ~/.some-config
```

### 6. 커맨드

```bash
mydesk link            # 심볼릭 링크 생성
mydesk unlink          # 심볼릭 링크 제거 + 백업 복원
mydesk diff            # 시스템 vs 레포 드리프트 감지
mydesk sync            # Brewfile, VS Code extensions 내보내기
mydesk setup           # 전체 프로비저닝 (새 맥)
mydesk install-shell   # MYDESK_CONFIG_DIR를 셸 프로파일에 설정
mydesk web             # Web UI 실행 (localhost:8080)
```

### 글로벌 플래그

```bash
mydesk --dry-run link     # 실제 변경 없이 미리보기
mydesk --verbose link     # 상세 출력
mydesk --no-color link    # 컬러 비활성화
mydesk --config-dir <path> link  # 설정 레포 경로 지정
```

## 환경변수

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `MYDESK_CONFIG_DIR` | 설정 레포 경로 | CWD에서 `links.conf` 탐색 |
| `MYDESK_REPOS` | `$REPOS` 변수 값 | `~/Repositories` |
| `NO_COLOR` | 컬러 비활성화 | - |

## 개발

```bash
mise install       # Go 설치
mise run build     # 빌드
mise run test      # 테스트
mise run install   # 로컬 설치
```
