# jgw 설계: jg 의 worktree 모드 형제 CLI

본 문서는 `jg` 의 형제 CLI 인 `jgw` 의 설계를 정의한다. `jgw` 는 git
저장소의 원본(main working tree) 과 linked worktree 들 사이를 빠르게
점프하기 위한 도구이며, `jg` 와 동일 바이너리를 공유하면서 호출 이름에
따라 동작 모드를 분기하는 busybox 패턴으로 구현한다. 같은 작업의 일환으로
저장소 전체 원칙 문서(`CLAUDE.md`) 에 "자동완성 최대 제공" 원칙을 함께
추가한다.

## 한 줄 정의

`jgw` 는 git 저장소의 원본 working tree 와 그에 연결된 linked worktree
들의 목록을 frecency 점수와 함께 fzf picker 로 보여주고, 사용자가
선택한 worktree 의 경로로 점프하도록 돕는 명령이다.

## 대상 사용자와 사용 맥락

본 저장소를 운영하는 1인 개발자가 자신의 일상 개발 환경에서 사용한다.
하나의 git 저장소에 여러 linked worktree 를 만들어 여러 브랜치를 동시에
열어 두고 작업하는 흐름이 잦으며, 그 worktree 들 사이를 키 입력 몇 번으로
오가는 것이 본 도구의 목적이다.

## 목표

- `jg` 와 동일한 점프 감각을 worktree 단위에서 제공한다.
- cwd 가 어디든 자연스럽게 동작하도록 입력 surface 를 최소화한다.
- `jg` 의 기존 동작과 데이터를 깨지 않는 순수 additive 추가가 되도록
  한다. `jg` 만 쓰는 사용자에게는 본 기능 도입이 보이지 않아야 한다.
- 셸 자동완성을 처음부터 제공한다. 본 저장소의 새 원칙("자동완성 최대
  제공") 의 첫 적용 사례로 둔다.

## 비목표

- worktree 자체의 생성·삭제·이름 변경 같은 라이프사이클 관리는 다루지
  않는다. 이는 `git worktree add` 등 git 본체의 책임이다.
- `jg` 의 frecency store schema 를 변경하지 않는다. worktree 점수는
  별도 파일로 분리해 보관한다.
- macOS·Linux 외 플랫폼 지원은 다루지 않는다. `jg` 가 이미 지원하는
  범위를 그대로 따른다.

## 외부 의존

- `git`: worktree 목록 조회(`git worktree list --porcelain`), 브랜치
  조회(`git symbolic-ref --short HEAD` 등), repo 식별(`git
  rev-parse --git-common-dir`).
- `fzf`: picker UI. `jg` 와 동일하게 외부 도구 가정.
- `jg` 의 frecency store: 흐름 b 의 repo 후보를 가져올 때 읽기 전용으로
  사용한다. 같은 바이너리이므로 함수 호출로 직접 접근한다.

## 아키텍처 — busybox 패턴

`jgw` 는 별도의 Go main 이나 별도 `apps/jgw/` 디렉토리를 두지 않는다.
`apps/jg/` 안의 한 바이너리에 진입점 분기를 추가하고, Homebrew formula
가 `bin.install_symlink "jg" => "jgw"` 로 심볼릭 링크를 함께 설치한다.
바이너리는 시작 시점에 `filepath.Base(os.Args[0])` 를 보고 jg 모드와
jgw 모드를 분기한다.

이 선택의 영구 근거는 다음과 같다. 본 저장소의 모노레포 컨벤션은 "한
도구 = `apps/<tool>/` 한 디렉토리, 도구 사이 코드 공유 금지" 이다.
`jgw` 가 `jg` 의 frecency store 코드를 읽어야 하는 이상 두 도구가 코드
또는 데이터 contract 를 공유할 수밖에 없다. 별 디렉토리로 분리하면
공유 패키지를 만들어야 하고 이는 컨벤션과 정면으로 충돌한다. busybox
패턴은 두 도구를 한 디렉토리 한 바이너리로 묶어 컨벤션을 자연스레
충족시키며, 빌드·CI·릴리스·버전 관리 모두 jg 한 도구 단위로만
관리하면 된다. 신규 인입자가 코드를 읽었을 때 argv0 분기는 UNIX 의
표준 관용구이므로 가독성 비용도 크지 않다.

## 사용자 흐름

`jgw` 를 인자 없이 호출하면 현재 작업 디렉토리(cwd) 를 기준으로 두 가지
흐름 중 하나로 자동 분기한다.

**흐름 a — 현 repo 의 worktree 점프**

cwd 가 git repo 안일 때 진입한다. `git worktree list --porcelain` 으로
해당 repo 의 원본과 linked worktree 들을 모두 모아 fzf picker 한 단계로
보여준다. 사용자가 항목을 선택하면 그 경로로 점프한다.

**흐름 b — repo 먼저 고르기**

cwd 가 git repo 밖이거나, 사용자가 `jgw <pattern>` 으로 명시 호출했을
때 진입한다. `jg` 의 frecency store 에서 repo 후보를 가져와 1단계 repo
picker 를 띄우고, 선택된 repo 에 대해 다시 worktree picker 를 띄운다.
2단계로 구성된 흐름이며, header 영역에 단계 표시를 둔다.

**빈 worktree 케이스**

선택된 repo 에 linked worktree 가 0개이고 원본만 있는 경우, picker 를
띄우지 않고 즉시 원본 경로로 점프한다. picker UI 비용을 1개짜리
선택지로 강요하지 않기 위한 결정이다.

## picker 표시 규약

picker 의 후보 라인은 path 한 줄짜리로만 채운다. 후보 라인 자체에는
브랜치 이름·원본 마커·접두 기호를 두지 않는다. 컨텍스트 정보는 fzf 의
주변 영역으로 분산한다.

**상단 header (multi-line)**

흐름 b 의 첫 단계에서는 `[1/2 repo 선택]` 한 줄을 둔다. 두 번째 단계
에서는 `[2/2 worktree 선택]` 한 줄과 그 위에 `원본: <path> (<branch>)`
한 줄을 추가해 어느 path 가 원본 working tree 인지 명시한다. 흐름 a
에서는 단계가 하나뿐이므로 `원본: <path> (<branch>)` 한 줄만 둔다.

**header-lines (비활성 행)**

사용자가 현재 위치한 worktree 가 후보군 안에 있으면 그 행을 fzf 의
`--header-lines=1` 기능으로 picker 맨 위에 비활성 행으로 노출한다.
`--header-lines` 행은 fzf 가 검색·선택 대상에서 자동으로 제외하므로
"보이지만 선택 불가" 가 트릭 없이 성립한다.

**preview 창**

focused 항목의 브랜치 이름, 마지막 커밋 제목, 마지막 커밋 시각을
preview 창에 표시한다. focused 가 바뀔 때마다 해당 path 에 대해 `git -C
<path> log -1 --format=...` 형태로 1회 호출한다. 후보가 수십~수백 개라도
picker 가 뜨는 시점엔 git 호출이 0번이고, focus 이동 시에만 한 항목당
1회 호출이 발생하므로 체감 지연이 거의 없다.

이 표시 규약의 영구 근거는 다음과 같다. 후보 라인을 path 만으로 둔 덕에
양 끝 정렬·터미널 너비 계산·텍스트 truncate 같은 표시 복잡도가
사라지고, 모든 라인이 균일해 사용자가 시각적으로 빠르게 훑을 수 있다.
브랜치 정보가 필요한 순간은 "지금 커서가 있는 한 항목" 뿐이라는
관찰에 부합한다.

## frecency 데이터 모델 — 분리 store

`jg` 가 보유하는 기존 store(repo 점수) 의 schema 는 변경하지 않는다.
`jgw` 는 별도 파일 `worktrees.json` (정확한 파일명·경로는 구현 plan
단계에서 확정) 을 두고 worktree path 단위의 점수를 보관한다.

- `jg` 점프 동작: repo store 의 해당 repo entry 만 점수를 가산한다.
- `jgw` 점프 동작: worktree store 의 해당 worktree entry 점수를
  가산하고, 동시에 repo store 의 parent repo entry 점수도 가산한다.
  두 번의 atomic rename 으로 처리한다.
- `jg` 만 쓰는 사용자에게는 `worktrees.json` 파일이 절대 생성되지 않으며
  `jg` 의 store 도 형태가 바뀌지 않는다.

분리 store 선택의 영구 근거는 다음과 같다. 단일 store 에 `type` 태그를
추가하는 안과 비교했을 때 분리 store 는 `jg` schema 를 건드리지 않아
순수 additive 가 되고, worktree 전용 메타데이터(예: 마지막으로 본
브랜치 캐시) 를 자유롭게 확장할 여지를 남긴다. 두 파일 갱신의 원자성
손실 위험은 atomic rename 두 번이 도중에 끊길 확률이 사실상 0이며,
끊겨도 frecency 가 1점 어긋날 뿐이라 사용자에게 보이는 차이가 없다.

## 셸 통합

기존 `jg init zsh` 와 `jg init bash` 출력이 두 셸 함수 `jg` 와 `jgw` 를
한 번에 정의하도록 확장한다. 사용자가 `.zshrc` 에 `eval "$(jg init zsh)"`
한 줄만 두면 양쪽 셸 함수가 등록되며, 각 함수는 `cd "$(command jg ...)"`
또는 `cd "$(command jgw ...)"` 형태로 실제 점프를 수행한다.

이 통합 위치 선택의 영구 근거는 다음과 같다. 한 바이너리이므로 init
명령의 책임도 한 군데로 모이는 것이 자연스럽다. 사용자에게도 "설치 후
한 줄만 추가" 라는 기존 jg 의 약속을 깨지 않는다.

## 자동완성

`apps/jg/completions/` 아래에 zsh 자동완성 `_jg`, `_jgw` 와 bash 자동
완성 `jg.bash`, `jgw.bash` 4개 파일을 둔다. 기존 `_jg` 와 `jg.bash` 는
원래의 동작을 유지하면서 등록 경로 차집합 보완 같은 자잘한 개선을 같이
반영한다. 새로 추가되는 `_jgw` 와 `jgw.bash` 는 다음 후보들을 보완한다.

- 1번째 인자: cwd 가 repo 안이면 비어 있어도 동작하므로 자동완성은 비워
  두어도 무방하다. cwd 가 repo 밖이면 jg frecency store 의 repo 목록을
  후보로 노출해 사용자가 흐름 b 의 1단계 repo 를 빠르게 좁힐 수 있게
  한다.
- 플래그: `-h`, `--help`, `-v`, `--version` 한정.

GoReleaser archive 의 `files:` 항목에 `completions/*` 를 추가해 tar.gz
에 4개 파일이 함께 들어가도록 한다. Homebrew formula 의 `install`
블록에서 각각 `zsh_completion.install` 과 `bash_completion.install` 로
설치한다.

## 버전 출력

두 도구는 동일 버전 번호를 공유한다. 한 바이너리이므로 자연스러우며,
release-please 의 manifest 도 `apps/jg` 한 패키지만 bump 하면 양쪽이
함께 올라간다. 출력 한 줄은 본 저장소의 version-conformance 규칙을 그대
로 따른다.

```
jg v0.5.0 © 2026 silee-tools
jgw v0.5.0 © 2026 silee-tools
```

`versionLine(name, version)` 순수 함수가 한 줄을 만들고, argv0 분기로
얻은 도구 이름을 그대로 name 슬롯에 넘긴다. version-conformance 게이트
는 `apps/*` 디렉토리를 순회하므로 `apps/jg` 한 곳만 검증하고 jgw 디렉
토리는 별도로 등록할 필요가 없다.

## README

루트 `README.md` 와 `docs/README_ko.md` 의 도구 표는 jg 와 jgw 를 한
행에 묶어 표기한다. 두 도구가 동일 바이너리이며 동일 설치·동일 셋업
이라는 사실을 표 한 행으로 분명히 보여 주기 위함이다.

| Tool | Language | Description |
|---|---|---|
| jg / jgw | Go | Frecency-based CLI for quickly jumping to git repositories (jg) and to worktrees within a repo (jgw). |

## 저장소 원칙 추가

루트 `CLAUDE.md` 의 "저장소 원칙" 섹션에 4번째 항목으로 다음을 추가한다.

> **자동완성 최대 제공**: 각 도구는 사용자 셸 환경에서 가능한 모든 입력
> 지점에 자동완성을 제공한다. 정적 후보(subcommand, 플래그) 는 물론 동적
> 후보(파일/디렉토리/도구 자체가 보유한 항목명) 도 셸이 표현할 수 있는 한
> 함께 보완한다. 옵션 비교 시 가산점 기준으로 사용한다.

동일 섹션 아래의 "새 도구 추가 체크리스트" 에도 자동완성 파일 작성을
한 항목으로 추가해, 신규 도구가 자동완성 누락 상태로 들어오지 않도록
한다.

## 수용 기준

본 설계가 구현되어 다음을 모두 만족할 때 작업이 완료된 것으로 본다.

- cwd 가 git repo 안에서 `jgw` 를 실행하면 그 repo 의 원본과 linked
  worktree 들이 fzf picker 에 path 한 줄짜리로 나열되고, 사용자가 항목
  을 고르면 셸 함수가 그 경로로 cd 한다.
- cwd 가 git repo 밖에서 `jgw` 를 실행하면 jg frecency store 의 repo
  목록이 1단계 picker 로 뜨고, repo 선택 후 그 repo 의 worktree picker
  가 2단계로 이어진다. 각 단계에 단계 표시 헤더가 보인다.
- 선택된 repo 에 linked worktree 가 0개면 picker 를 띄우지 않고 즉시
  원본으로 점프한다.
- 현재 위치한 worktree 가 후보군에 포함되면 picker 맨 위 비활성 행에
  표시되며, 그 행은 검색·선택 모두 불가능하다.
- focused 항목 변경 시 preview 창에 그 worktree 의 브랜치, 마지막 커밋
  제목, 마지막 커밋 시각이 나타난다.
- `jgw` 점프가 worktree store 와 jg repo store 양쪽 점수를 동시에 올린
  다. `jg` 점프는 repo store 만 올린다.
- `jg init zsh` 한 줄로 `jg` 와 `jgw` 셸 함수가 모두 정의된다.
- `jg -v` / `jgw -v` 가 본 저장소의 version-conformance 형식을 만족한다.
- zsh 와 bash 양쪽에서 `jg`, `jgw` 자동완성이 동작한다.
- `jg` 만 쓰는 사용자의 store 파일 형태가 본 변경 전과 동일하다(추가
  되는 worktree store 파일은 jgw 첫 사용 전까지 생성되지 않는다).
- 루트 `CLAUDE.md` 에 자동완성 원칙이 4번째 항목으로 추가되고, 신규 도구
  체크리스트에도 자동완성 항목이 등장한다.

## 검증 방식

본 저장소의 TDD 원칙(`~/.claude/tdd-principles.md`) 을 따라 단위 테스트
는 Red → Green → Refactor 사이클로 작성한다. 단위 테스트가 다루기 어려운
실제 셸 통합·실제 git worktree·실제 fzf 키 입력 경로는 1회성 E2E 로 PR
본문에 증거를 첨부한다. 최소 다음 5건의 E2E 케이스를 수동으로 통과시킨
다.

1. linked worktree 가 0개인 repo 에서 `jgw` 실행 → picker 없이 원본
   경로로 점프하는 것을 셸 함수의 `pwd` 변화로 확인한다.
2. linked worktree 가 2개 이상인 repo 안에서 `jgw` 실행 → 흐름 a 의
   1단계 picker 가 뜨고, header 에 단계·원본 정보가 보이며, 선택 후
   `pwd` 가 해당 worktree 로 변경되는 것을 확인한다.
3. cwd 가 repo 밖일 때 `jgw` 실행 → 흐름 b 의 2단계 picker 가 차례로
   뜨며, header 단계 표시가 `1/2` → `2/2` 로 진행되는 것을 확인한다.
4. 현재 worktree 안에서 `jgw` 실행 → picker 맨 위에 비활성 행으로
   현재 worktree 가 나타나고, 그 행을 키보드로 선택하려 해도 cursor 가
   넘어가지 않는 것을 확인한다.
5. focus 를 worktree A 에서 B 로 옮겼을 때 preview 창 내용이 A 의 브랜치/
   커밋 정보에서 B 의 정보로 갱신되는 것을 확인한다.

각 E2E 통과 증거는 셸 캡처(필요 시 스크린샷) 로 PR 본문에 첨부한다.

## 작업 분해

본 설계를 구현 plan 으로 옮길 때 사용할 큰 단위 분해는 다음과 같다.
세부 task 와 의존 관계는 구현 plan 에서 별도로 정의한다.

- **A. 저장소 원칙 추가**: 루트 `CLAUDE.md` 에 "자동완성 최대 제공"
  원칙과 신규 도구 체크리스트 한 항목을 추가한다.
- **B. jgw 본체 구현**: `apps/jg/` 내부에 argv0 분기, worktree 목록
  수집, picker 호출, worktree store 의 read/write 와 frecency 가산
  로직, 빈 worktree 케이스의 즉시 점프 동작, preview 명령을 구현한다.
  단위 테스트는 store 갱신과 흐름 a/b 분기 결정 로직을 중심으로 둔다.
- **C. 셸 통합과 자동완성**: `jg init` 출력에 jgw 함수 정의를 추가하고,
  zsh/bash 자동완성 4종 파일을 작성한다.
- **D. 배포 경로 갱신**: GoReleaser archive `files:` 에 `completions/*`
  를 추가하고, Homebrew formula 에 자동완성 설치 줄과 jgw 심볼릭 링크
  설치 줄을 추가한다. 루트 README 와 한국어 README 의 도구 표를 갱신
  한다.
- **E. 수용 기준 검증**: 본 문서의 "수용 기준" 과 "검증 방식" 에 정의된
  단위 테스트 통과 + 5건의 1회성 E2E 통과 + 증거 첨부.

각 단위는 별도 commit 으로 끊는다.
