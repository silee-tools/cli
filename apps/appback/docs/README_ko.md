# appback

[English (영어)](../README.md)

Mac 앱 설정 백업 및 복원 CLI 도구.

macOS 앱의 설정, 환경설정, 키체인 항목을 [gum](https://github.com/charmbracelet/gum) 기반 인터랙티브 터미널 UI로 백업하고 복원합니다.

## 기술 스택

- Bash
- [gum](https://github.com/charmbracelet/gum) — 인터랙티브 터미널 UI
- [bats](https://github.com/bats-core/bats-core) — 테스트 프레임워크

## 시작하기

```bash
# 의존성 설치
brew install gum

# 클론 및 사용
git clone https://github.com/silee9019/appback.git
cd appback
./appback --help
```

## 사용법

```bash
# 지원 앱 목록
./appback list

# 앱 설정 내보내기
./appback export dia

# 앱 설정 가져오기
./appback import dia ~/Downloads/dia-backup-2026-04-08.tar.gz

# 인터랙티브 모드 (인자 없이)
./appback export
./appback import
```
