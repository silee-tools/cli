# git-tidy

순수 zsh 플러그인. upstream 이 사라진(`[gone]`) 로컬 브랜치를 보호 규칙(현재·기본
브랜치, worktree, 최근 커밋)에 따라 안전하게 정리한다. Go 코드가 없으며
`git-tidy.plugin.zsh` 단일 파일이 전부다.

## Development

`mise tasks` 로 사용 가능한 태스크 확인. 자주 쓰는 것:

- `mise run shell-check` — `zsh -n` 문법 검사(하드 게이트) + `shellcheck -s bash`(보조). zsh 전용 파라미터 확장은 `# shellcheck disable=SC2296` 로 false positive 만 억제했고 로직은 손대지 않는다.
- `mise run install` / `mise run uninstall` — `${XDG_DATA_HOME:-$HOME/.local/share}/git-tidy/` 에 플러그인 설치·제거.

CI(`.github/workflows/git-tidy-ci.yml`)는 ubuntu 러너에서 `zsh` 만 추가 설치하고
`mise run shell-check` 를 돈다(shellcheck 는 ubuntu 러너 기본 탑재).

## 릴리스

`git-tidy/v<MAJOR>.<MINOR>.<PATCH>` 태그. 루트 `release-please.yml` 이 자동
처리한다. 순수 zsh 도구라 `.goreleaser.yaml` 은 `builds: [{skip: true}]` 로 Go
빌드를 끄고 `archives` 의 `meta: true` 로 `git-tidy.plugin.zsh` 만 담은
`git-tidy-v<버전>.tar.gz` + `checksums.txt` 만 만든다. 그 파일명이
homebrew-tap `Formula/git-tidy.rb` 의 URL 파일명과 정확히 일치해야
sha256 자동 갱신이 동작한다.
