# git-tidy

로컬 git 브랜치를 일괄 정리하는 명령줄 도구. 작업이 끝났거나 오래 방치된 브랜치를
자동으로 찾아 보여주고, 사용자가 체크박스로 확인하면 한 번에 삭제한다.

삭제 후보 신호(작업이 끝났다는 양성 근거)와 보호 규칙(절대 건드리면 안 되는 구조적
조건) 두 층을 조합하는 하이브리드 모델이다. 신호에 걸리고 보호 규칙에 걸리지 않은
브랜치만 정리 대상이 된다.

## 설치

```bash
brew install silee-tools/tap/git-tidy
```

또는 소스에서 직접 빌드:

```bash
cd apps/git-tidy
mise run build
```

## 사용

```bash
git-tidy                   # dry-run (삭제 대상만 표시, 기본 동작)
git-tidy --run             # 체크박스로 확인 후 실제 삭제
git-tidy --run --no-tui    # 줄 기반 선택으로 실제 삭제
git-tidy --stale-days=N    # stale 판정 창을 N일로 변경 (기본 20일)
git-tidy --no-fetch        # git fetch --prune 단계 건너뛰기
git-tidy --version         # 버전 출력 (-v 동일)
git-tidy --help            # 사용법 출력 (-h 동일)
gtidy                      # git-tidy 와 동일
gtidy!                     # git-tidy --run 과 동일
```

`gtidy` 와 `gtidy!` 는 Homebrew formula 와 `mise run install` 이 함께 설치하는 단축
명령이다.

선택 화면에서는 `↑↓`/`jk` 이동, `space` 토글(그룹 헤더에서는 그룹 일괄 토글),
`a` 전체 토글, `enter` 삭제, `esc` 취소를 쓴다.

## 동작 방식

기본 동작은 dry-run 이라, `--run` 을 명시하기 전에는 어떤 브랜치도 삭제하지 않는다.
실행 순서는 다음과 같다.

1. `--no-fetch` 가 없으면 `git fetch --prune` 으로 원격 기준 추적 정보를 먼저
   갱신한다.
2. 삭제 후보 신호 세 가지를 검사한다. 하나라도 해당하면 그 브랜치가 삭제 후보다.
   - **`[gone]`**: upstream 추적 브랜치가 사라진 상태. squash merge 워크플로에서
     PR 이 머지된 뒤 원격 브랜치가 삭제되면 이 상태가 된다.
   - **merged**: `git branch --merged <base>` 로 확인한다. fast-forward 나
     merge commit 으로 합쳐진 브랜치가 대상이다.
   - **stale**: 마지막 커밋 또는 merge-base 가 stale 판정 창(기본 20일)보다
     오래된 경우다.
3. 보호 규칙을 적용한다. 다음 중 하나라도 해당하면 후보에서 제외한다.
   - 현재 체크아웃된 브랜치
   - 자동 감지한 base 브랜치(main·master·trunk 등)
4. 남은 브랜치가 삭제 대상이다. dry-run 이면 삭제 사유(`gone` → `merged` →
   `stale`)별로 묶어 목록만 출력한다. `--run` 이면 같은 그룹 구조의 선택 화면을
   띄우는데, 가장 확실한 후보인 `[gone]` 만 기본 체크된 상태로 시작하고 사용자가
   나머지를 직접 고른다. `stale` 항목에는 마지막 커밋 기준 경과 일수가, worktree 에
   물린 브랜치에는 worktree 이름이 함께 표시된다.

다른 worktree 에 체크아웃된 브랜치가 삭제 대상이 되면, worktree 를 먼저 제거한 뒤
브랜치를 삭제한다. worktree 에 커밋하지 않은 변경이 있으면 해당 브랜치를 실패로
보고하고 건너뛴다.

## 환경 변수

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `GIT_TIDY_STALE_DAYS` | stale 판정 창(일). `--stale-days=N` 으로 호출 시점에 덮어쓸 수 있다. | `20` |

## 개발

```bash
mise run build      # 빌드
mise run test       # 테스트 실행
mise run lint       # 린터
mise run fmt-check  # gofmt 검사 (CI 동일)
```
