# jgw worktree picker 표시 개선 설계

## 개요

`jgw` 는 한 저장소의 원본 working tree 와 linked worktree 사이를 fzf picker 로 골라
이동하게 해 주는 명령이다. 지금 이 picker 는 후보 worktree 를 화면에 보여줄 때 각 행에
단축된 폴더 경로(예: `~/repos/acme-app/.worktrees/ABC-101-login-timeout`)
를 그대로 출력한다. 사용자는 이동할 worktree 를 고를 때 폴더 경로보다 worktree 이름과
어떤 브랜치가 올라가 있는지를 먼저 보고 싶어 한다. 또한 그 텍스트를 fzf 의 fuzzy
검색으로 좁힐 수 있어야 한다.

이 작업은 worktree picker 단계에서 화면에 표시하는 텍스트를 worktree 이름 중심으로
바꾼다. 화면 표시만 바뀌고, 사용자가 항목을 고른 뒤 셸 함수가 받아 `cd` 하는 값은
지금과 똑같이 worktree 의 실제 경로다. 즉 보이는 것과 돌아가는 값을 분리한다.

## 대상 사용자

`jgw` 로 git worktree 사이를 자주 오가는 본인. worktree 디렉토리 이름을 브랜치명에서
따오는 운영 패턴(예: 디렉토리 `ABC-101-login-timeout`, 브랜치 `feature/ABC-101-login-timeout`)을 쓴다.

## 목표

- worktree picker 의 각 행을 폴더 경로 대신 worktree 이름 중심으로 보여준다.
- 이름과 브랜치명이 사실상 같은 흔한 경우에 같은 문자열을 두 번 보여주지 않는다.
- picker 의 fuzzy 검색이 화면에 보이는 텍스트(이름·브랜치)에 걸린다.
- 항목을 고르면 지금과 똑같이 worktree 실제 경로가 출력되어 셸 `cd` 와 frecency 기록이
  그대로 동작한다.

## 비목표

- 저장소(repo) 를 고르는 picker 단계의 표시 방식은 바꾸지 않는다. 저장소는 브랜치라는
  개념이 없으므로 경로 표시가 적절하다.
- worktree 의 dirty 여부, ahead/behind 카운트 같은 새 상태 정보를 행에 추가하지 않는다.
- 정렬 순서, frecency 점수 계산, 흐름 분기(저장소 안/밖) 규칙은 손대지 않는다.

## 표시 형식

worktree picker 의 한 행은 worktree 이름을 먼저 보여주고, 필요할 때만 브랜치를 덧붙인다.

- **이름**: worktree 경로의 마지막 디렉토리 이름(basename). 원본 working tree 는 보통
  저장소 디렉토리 이름이 되고, linked worktree 는 그 worktree 디렉토리 이름이 된다.
- **원본 표시**: 원본 working tree(main) 행 앞에는 `▸ ` 마커를 붙여 한눈에 구분한다.
  나머지 행은 같은 폭의 공백 `  ` 으로 들여써 세로로 정렬한다.
- **브랜치 덧붙임 규칙**: 브랜치 이름의 마지막 세그먼트(basename)가 worktree 이름과 같으면
  브랜치를 생략하고 이름만 보여준다. 다르면 이름 뒤에 공백 두 칸을 두고 브랜치 전체 이름을
  덧붙인다. 이 규칙 덕분에 디렉토리 `ABC-101-login-timeout` 과 브랜치
  `feature/ABC-101-login-timeout` 처럼 basename 이 같은 흔한 경우에는 브랜치가 깔끔히 숨고,
  디렉토리 이름과 브랜치가 실제로 다를 때만 브랜치가 드러난다.
- **브랜치 없는 worktree(detached HEAD)**: 브랜치 자리에 `(detached)` 를 덧붙인다.

브랜치를 덧붙일 때 이름 컬럼을 최장 이름 기준으로 패딩해 정렬하지는 않는다. 브랜치가
덧붙는 행이 보통 소수라, 패딩하면 그 한두 행 때문에 모든 이름이 멀리 밀려 오히려 읽기
불편하기 때문이다. 브랜치는 이름 바로 뒤에 공백 두 칸으로 잇는다.

worktree 디렉토리 이름을 브랜치 basename 에서 따오는 저장소의 worktree 목록에 이 형식을
적용하면 다음과 같다(원본은 `main` 브랜치, linked worktree 는 `feature/<티켓>-<요약>` 브랜치를
같은 이름의 디렉토리에 둔 예시).

```
▸ acme-app  main
  ABC-101-login-timeout
  ABC-102-upload-retry
  ABC-103-export-pagination
  ABC-104-oauth-refresh
```

원본만 디렉토리 이름(`acme-app`)과 브랜치(`main`)의 basename 이 달라 브랜치가
덧붙고, 나머지는 이름과 브랜치 basename(`feature/ABC-101-login-timeout` → `ABC-101-login-timeout`)이
같아 이름만 남는다.

## fuzzy 검색과 선택 반환

화면에 보이는 텍스트(이름과, 덧붙은 경우 브랜치)가 fzf 의 fuzzy 검색 대상이 되어야 하고,
사용자가 고른 뒤에는 그 worktree 의 실제 경로가 출력되어야 한다. 보이는 값과 돌려주는
값이 다르므로 둘을 잇는 방법이 필요하다.

이를 위해 **유일 인덱스를 숨김 필드로 동행**시킨다. picker 에 넘기는 입력의 각 줄을
`<인덱스>\t<표시텍스트>` 두 필드로 만들고, fzf 에 탭을 구분자로 지정해 인덱스 필드를
화면에서 숨기고 검색 대상에서도 제외한다. 사용자가 한 줄을 고르면 fzf 가 그 줄을 돌려주고,
맨 앞 인덱스 필드를 읽어 worktree 목록 슬라이스의 같은 자리 원소를 찾아 그 경로를 출력한다.

표시텍스트는 이름이 겹치거나 브랜치가 같아 우연히 동일해질 수 있지만, **반환 식별에는
표시텍스트가 아니라 줄마다 유일한 인덱스를 쓰므로** 어떤 줄을 골랐는지 항상 정확히 되짚을
수 있다. 최종 반환 경로는 인덱스로 찾은 슬라이스 원소에서 가져오므로, 표시텍스트가 겹쳐도
반환이 어긋나지 않는다.

## 적용 범위와 코드 경계

worktree picker 는 두 자리에서 쓰인다. 저장소 안에서 `jgw` 를 부른 흐름(한 단계)과,
저장소 밖에서 부른 흐름의 두 번째 단계다. 두 자리 모두 이 새 표시 형식을 쓴다. 저장소
밖 흐름의 **첫 단계인 저장소 picker 는 지금처럼 경로를 표시**한다.

현재는 저장소 picker 와 worktree picker 가 같은 함수(`RunWorktreePicker`)를 공유하며,
둘 다 "경로 문자열 목록"을 입력으로 받아 화면에 단축 경로를 그린다. 이번 변경으로 두
용도의 입력이 달라진다. worktree picker 는 이름·브랜치·경로를 함께 가진 worktree 목록을
받아야 하고, 저장소 picker 는 지금처럼 경로 목록만 받으면 된다. 따라서 worktree 전용
입력을 받는 진입점을 새로 두어, 경로 목록을 받는 기존 진입점과 분리한다. 기존 진입점은
저장소 picker 가 계속 쓴다.

현재 위치한 worktree 를 picker 맨 위에 고정해 보여주는 동작(흐름 안쪽에서 `jgw` 를
부를 때, 지금 있는 worktree 를 헤더 줄로 고정)도 같은 표시 형식을 따른다. 그 고정 줄
역시 이름 중심 표기로 그린다.

## preview 패널

worktree picker 에서는 preview 패널을 **띄우지 않는다**. 지금은 focus 된 항목의 브랜치,
마지막 커밋 제목, 마지막 커밋 시각을 오른쪽 preview 패널에 보여주지만, 이제 행 자체가
worktree 이름과 (다를 때) 브랜치를 보여주므로 picker 가 한눈에 읽히고 preview 가 군더더기가
된다. preview 를 없애면 화면이 단순해지고, 인덱스 동행 입력도 경로 필드 없이
`<인덱스>\t<표시텍스트>` 두 필드로 충분해진다. worktree picker 의 새 진입점은 preview
옵션을 fzf 에 넘기지 않는다.

단, 지금 worktree picker 가 쓰는 preview 명령은 **저장소(repo) picker 단계도 함께 쓴다**.
저장소 밖에서 `jgw` 를 부르는 흐름의 1단계 저장소 picker 는 worktree 단계와 같은 picker
진입점을 공유하며, 그 진입점이 focus 된 저장소의 브랜치·커밋·status 를 preview 로 보여준다.
저장소 picker 의 preview 는 이 변경의 범위 밖이므로 그대로 둔다. 따라서 preview 명령
정의 자체는 제거하지 않고, worktree 단계만 새 진입점으로 옮겨 preview 를 끄는 방식으로
나눈다.

## 주요 시나리오

1. **저장소 안에서 worktree 이동**: 어떤 worktree 안에서 `jgw` 를 부르면, 그 저장소의
   worktree 들이 이름 중심으로 picker 에 나온다. 이름이나 브랜치 일부를 입력해 후보를
   좁히고, 고른 worktree 로 이동한다.
2. **저장소 밖에서 worktree 로 점프**: 저장소 밖에서 `jgw` 를 부르면 저장소 picker(경로
   표시)가 먼저 뜨고, 저장소를 고르면 그 저장소의 worktree picker(이름 중심)가 이어
   뜬다. 두 단계를 거쳐 원하는 worktree 로 이동한다.
3. **디렉토리 이름과 브랜치가 다른 worktree**: worktree 디렉토리 이름과 올라간 브랜치의
   basename 이 다르면, 그 행에는 이름 뒤에 브랜치가 덧붙어 어떤 브랜치인지 바로 보인다.

## 수용 기준

- worktree picker 의 각 행이 폴더 경로가 아니라 worktree 이름 중심으로 표시된다.
- 브랜치 basename 이 worktree 이름과 같으면 브랜치가 생략되고, 다르면 이름 뒤에 브랜치가
  덧붙으며, detached worktree 는 `(detached)` 가 덧붙는다.
- 원본 working tree 행에 `▸` 마커가 붙는다.
- picker 의 fuzzy 검색이 화면에 보이는 이름·브랜치 텍스트에 걸리고, 숨김 인덱스 필드는
  검색에 끼어들지 않는다.
- 항목을 고르면 worktree 실제 경로가 출력되어 셸 `cd` 와 frecency 기록이 지금과 똑같이
  동작한다.
- 저장소 밖 흐름의 첫 단계인 저장소 picker 는 지금처럼 경로를 표시한다.
- worktree picker 에는 preview 패널이 더 이상 뜨지 않는다.
- 저장소 picker 의 preview 패널은 지금처럼 동작한다(이 변경의 범위 밖).

## 외부 의존

- `fzf`: picker 렌더링과 fuzzy 검색. worktree picker 는 탭 구분자로 인덱스 필드를 숨기고
  표시텍스트 필드만 보여주며 preview 패널은 쓰지 않는다.
- `git worktree list --porcelain`: worktree 경로·브랜치·원본 여부를 구조화해 읽는 원천.

## 결정과 근거

- **보이는 값과 돌려주는 값을 인덱스로 잇는다**: worktree 의 표시 텍스트는 이름·브랜치가
  겹쳐 동일해질 수 있어, 표시 텍스트를 키로 경로를 되짚으면 충돌 위험이 있다. 줄마다
  유일한 인덱스를 숨김 필드로 동행시키면 어떤 줄을 골랐는지 항상 정확히 되짚을 수 있고,
  경로에 들어갈 수 있는 특수 문자도 신경 쓸 필요가 없다.
- **이름 컬럼을 패딩 정렬하지 않는다**: 브랜치가 덧붙는 행은 보통 소수라, 최장 이름
  기준으로 패딩하면 그 한두 행 때문에 모든 이름이 멀리 밀려 읽기 불편하다. 브랜치는
  이름 바로 뒤에 공백 두 칸으로 잇는다.
- **저장소 picker 는 경로 표시를 유지한다**: 저장소는 브랜치 개념이 없어 이름 중심
  표기가 worktree 만큼의 이점을 주지 않고, 경로가 저장소를 식별하는 자연스러운 단위다.
