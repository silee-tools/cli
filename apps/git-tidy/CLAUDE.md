# git-tidy

Go 바이너리로 빌드되는 명령줄 도구. jg·totp 와 동일한 구조다.

## 구조

```
apps/git-tidy/
├── cmd/git-tidy/       # 진입점 (main.go)
├── internal/
│   ├── gitx/           # git 호출 (for-each-ref, worktree list, branch 조작)
│   ├── classify/       # 삭제 후보 신호 분류 + 보호 규칙 적용 (순수 함수)
│   └── pick/           # 다중 선택 모델 (체크박스 TUI + 줄 기반 선택 공유)
├── completions/
│   ├── _git-tidy       # zsh 자동완성
│   └── git-tidy.bash   # bash 자동완성
└── .goreleaser.yaml    # Go 바이너리 빌드 설정 (darwin + linux, amd64 + arm64)
```

## Development

`mise tasks` 로 사용 가능한 태스크 확인. 자주 쓰는 것:

- `mise run build` — Go 바이너리 빌드
- `mise run test` — `go test ./...` 실행
- `mise run lint` — golangci-lint 실행
- `mise run fmt-check` — `gofmt` 형식 검사 (CI 동일)

CI(`.github/workflows/git-tidy-ci.yml`)는 `fmt-check` → `lint` → `test` → `build`
순으로 실행한다.

## 릴리스

`git-tidy/v<MAJOR>.<MINOR>.<PATCH>` 태그. 루트 `release-please.yml` 이 자동 처리한다.
`.goreleaser.yaml` 가 darwin/linux × amd64/arm64 네 조합의 바이너리를 빌드해
GitHub Release 에 첨부한다. homebrew-tap `Formula/git-tidy.rb` 의 sha256/version 은
후속 step 이 자동 갱신한다.

## 정리 모델 요약

`classify` 패키지가 핵심 판단 로직을 담는다. 삭제 후보 신호(`[gone]` / merged /
stale) 와 보호 규칙(현재 브랜치, base 브랜치) 두 층으로 분류한다. 이 패키지는
git 호출에 의존하지 않는 순수 함수로 구성되며, 단위 테스트로 검증한다.

선택 화면은 `pick` 패키지가 담당한다. `classify` 가 신호 순위(gone → merged →
stale)로 정렬한 결과를 받아, `gone` 만 기본 체크한 그룹형 선택 모델을 만든다.
bubbletea 기반 TUI 와 줄 기반 폴백이 같은 모델을 공유하며, 그룹 헤더에서 그룹 일괄
토글을 지원한다. TUI 의 상태 전이(`Update`)는 순수 함수로 분리해 단위 테스트한다.
