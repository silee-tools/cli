# git-tidy absorbed 브랜치 설계

git-tidy 는 로컬 브랜치를 정리할 때 이미 끝난 작업을 더 잘 찾아야 한다. 지금의
`merged` 기준은 브랜치 커밋이 `main` 같은 base 브랜치에 그대로 들어간 경우만 잡는다.
스쿼시 머지처럼 커밋 모양이 바뀌거나, 작은 문서 변경이 더 큰 작업 안에 흡수된 경우에는
사람 눈에는 끝난 브랜치로 보이지만 도구는 삭제 후보로 보여주지 못한다.

이 설계는 그런 브랜치를 `absorbed` 라는 별도 후보로 보여준다. `absorbed` 는 자동 삭제할
확정 근거가 아니라, 사용자가 검토하기 좋은 강한 정황이다. 따라서 `gtidy!` 선택 화면에
나오지만 기본 체크는 해제한다.

## 목표

- 스쿼시 머지되었거나 더 큰 작업에 흡수된 것으로 보이는 로컬 브랜치를 삭제 후보로
  보여준다.
- `merged` 와 `absorbed` 를 구분해, Git 이 확실히 증명한 후보와 정황상 강한 후보를
  사용자가 다르게 판단할 수 있게 한다.
- dry-run, 체크박스 TUI, 줄 기반 선택 화면이 같은 후보 순서와 같은 설명 문구를 사용한다.

## 비목표

- 원격 브랜치를 삭제하지 않는다.
- GitHub PR 상태나 Jira 상태를 조회하지 않는다. git-tidy 는 로컬 git 정보만 사용한다.
- `absorbed` 후보를 기본 선택하지 않는다. 사용자가 직접 체크해야 삭제된다.
- 브랜치가 연결된 worktree 의 dirty 상태를 새로 검사하지 않는다. worktree 에 체크아웃된
  브랜치는 작업 중일 수 있으므로 `absorbed` 로 보지 않는다.

## 후보 종류

삭제 후보의 표시 순서는 `gone`, `merged`, `absorbed`, `stale` 로 둔다.

- `gone` 은 upstream 추적 브랜치가 사라진 로컬 브랜치다.
- `merged` 는 base 브랜치에 브랜치 tip 커밋이 그대로 들어간 로컬 브랜치다.
- `absorbed` 는 같은 Jira 티켓의 더 최신 작업이 base 브랜치에 있고, 지금 어떤 worktree
  에서도 작업 중이지 않은 로컬 브랜치다.
- `stale` 은 마지막 커밋 또는 분기점이 stale 기준일보다 오래된 로컬 브랜치다.

`absorbed` 는 `merged` 뒤에 둔다. `merged` 는 Git 이 히스토리로 증명한 결과이고,
`absorbed` 는 강한 정황이지만 사용자의 검토가 필요한 결과이기 때문이다. `stale` 보다는
앞에 둔다. 오래되었다는 사실만 있는 브랜치보다, 같은 티켓의 더 최신 base 커밋이 있다는
브랜치가 더 구체적인 삭제 근거를 갖기 때문이다.

## absorbed 판정 기준

브랜치는 다음 조건을 모두 만족할 때 `absorbed` 후보가 된다.

1. 브랜치 tip 커밋 제목에서 Jira 티켓 ID 를 찾을 수 있다.
2. base 브랜치에 같은 Jira 티켓 ID 를 가진 더 최신 커밋이 있다.
3. 브랜치가 어떤 worktree 에도 체크아웃되어 있지 않다.
4. 브랜치가 `gone` 또는 `merged` 로 이미 분류되지 않았다.
5. 브랜치가 현재 브랜치나 base 브랜치 보호 규칙에 걸리지 않는다.

Jira 티켓 ID 는 `ABC-1375` 처럼 영문 대문자와 숫자, 하이픈, 숫자로 된 토큰을 대상으로
한다. 이 패턴은 기존 저장소의 커밋 제목에서 작업 단위를 식별하는 데 충분히 구체적이다.

base 브랜치의 관련 커밋은 브랜치 tip 커밋보다 committer date 가 더 최신이어야 한다.
같은 티켓 ID 가 있더라도 base 커밋이 더 오래된 경우에는 그 브랜치 작업이 흡수되었다고
보지 않는다.

## 화면 표시

각 후보 그룹은 헤더 아래에 짧은 설명을 보여준다. TUI 에서는 설명 줄을 흐리게 표시한다.
dry-run 과 줄 기반 선택에서는 같은 문구를 들여쓰기된 일반 텍스트로 보여준다.

```text
[gone]
  upstream 추적 브랜치가 사라진 로컬 브랜치

[merged]
  base 브랜치에 브랜치 커밋이 그대로 들어간 로컬 브랜치

[absorbed]
  같은 Jira 티켓의 더 최신 base 커밋이 있고, 지금 worktree 에서 작업 중이지 않은 로컬 브랜치

[stale]
  마지막 커밋 또는 분기점이 stale 기준일보다 오래된 로컬 브랜치
```

`absorbed` 항목에는 가능하면 근거가 된 base 커밋의 짧은 해시와 제목을 함께 보여준다.
예시는 다음과 같다.

```text
[absorbed]
  같은 Jira 티켓의 더 최신 base 커밋이 있고, 지금 worktree 에서 작업 중이지 않은 로컬 브랜치
    claude/example-absorbed-branch  (base: 9a640b52f [ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서)
```

근거 커밋 제목은 너무 길면 화면 단계에서 줄인다. 분류 결과에는 원문을 보존하고,
렌더링에서만 줄이는 방식으로 둔다.

## 데이터 흐름

`gitx` 는 base 브랜치의 커밋 제목, 짧은 해시, committer date 를 읽어 `classify` 에
전달한다. `classify` 는 git 명령을 직접 호출하지 않는 순수 함수 상태를 유지한다.

`classify.Input` 은 base 브랜치 커밋 목록을 입력으로 받는다. `classify.Result` 는
`absorbed` 후보일 때 근거 커밋 정보를 담는다. dry-run, TUI, 줄 기반 선택은 같은
`classify.Result` 를 받아 같은 순서로 렌더링한다.

신호별 설명 문구는 한 곳에 둔다. 출력 계층이 각자 문구를 하드코딩하지 않게 해,
dry-run 과 선택 화면의 설명이 어긋나지 않도록 한다.

## 테스트

구현은 다음 회귀 테스트를 먼저 추가한 뒤 진행한다.

- `classify` 테스트는 같은 Jira 티켓 ID 를 가진 더 최신 base 커밋이 있고 worktree 에
  물려 있지 않은 브랜치가 `absorbed` 로 분류되는지 확인한다.
- `classify` 테스트는 worktree 에 체크아웃된 브랜치가 같은 티켓 ID 를 가져도
  `absorbed` 로 분류되지 않는지 확인한다.
- `classify` 테스트는 base 의 관련 커밋이 브랜치 tip 보다 오래되면 `absorbed` 로
  분류되지 않는지 확인한다.
- `classify` 테스트는 후보 순서가 `gone`, `merged`, `absorbed`, `stale` 로 유지되는지
  확인한다.
- dry-run 출력 테스트는 각 그룹 헤더 아래에 설명 문구가 표시되는지 확인한다.
- 선택 모델 테스트는 `absorbed` 후보가 기본 체크되지 않는지 확인한다.

## 완료 기준

- `apps/git-tidy` 에서 프로젝트의 Go 테스트 전체가 통과한다.
- `large-repo fixture` 에서 dry-run 을 실행했을 때 `claude/example-absorbed-branch` 같은
  브랜치가 `absorbed` 그룹에 표시된다.
- `gtidy!` 선택 화면에서 `absorbed` 그룹은 보이지만 기본 체크되어 있지 않다.
