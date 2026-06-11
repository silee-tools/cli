# git-update-default

현재 위치가 속한 git 저장소를 원격 default branch 의 최신 상태로 전환하는 명령줄 도구.
저장소 안 어느 하위 경로에서 실행해도 동작한다.

## 동작

1. 현재 위치가 git 저장소인지, origin 원격이 있는지 확인한다.
2. git fetch origin --prune 으로 원격 최신을 받는다.
3. default branch 를 main → master → gh(GitHub default) → origin/HEAD 순으로 정한다.
4. 커밋되지 않은 변경이 있으면 파일 목록을 보여주고 stash / force / 취소(기본값) 를 묻는다.
5. default branch 로 전환하고 origin/<default> 최신까지 fast-forward 한다.
   갈라져 fast-forward 가 불가능하면 경고만 하고 멈춘다(강제하지 않음).

## 사용

    git update-default          # 또는 git-update-default
    git update-default --stash  # dirty 일 때 묻지 않고 stash 후 진행
    git update-default --force  # dirty 일 때 묻지 않고 추적 변경 폐기 후 진행

## 설치

Homebrew tap(silee-tools/homebrew-tap)으로 설치하거나, 개발 빌드는
mise run install 로 ~/.local/bin 에 둔다.
