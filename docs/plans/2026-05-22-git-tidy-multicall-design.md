# git-tidy multi-call — gtidy / gtidy! 단축 명령

git-tidy 바이너리가 `gtidy` 와 `gtidy!` 라는 두 추가 이름으로도 호출될 수 있게
한다. 구현 작업은 이 설계를 단일 기준으로 삼는다.

## 한 줄 정의

`gtidy` 는 `git-tidy` 와 동일하게, `gtidy!` 는 `git-tidy --run` 과 동일하게
동작하는 단축 명령이다. 셸 별칭이 아니라 git-tidy 바이너리 자체가 제공하는
추가 명령 이름이다.

## 동작

- `git-tidy` 로 호출하면 현재 동작이 그대로다.
- `gtidy` 로 호출하면 `git-tidy` 와 완전히 같다. 인자도 그대로 받는다.
- `gtidy!` 로 호출하면 `git-tidy --run` 과 같다. `gtidy!` 에 함께 준 다른
  인자는 `--run` 뒤에 그대로 이어진다. 예를 들어 `gtidy! --no-tui` 는
  `git-tidy --run --no-tui` 와 같다.
- `-v` / `--version` 은 호출된 이름을 그대로 출력한다. `gtidy -v` 는
  `gtidy v<버전> © 2026 silee-tools`, `gtidy! -v` 는
  `gtidy! v<버전> © 2026 silee-tools` 를 출력한다. `git-tidy -v` 의 출력은
  바뀌지 않는다.

## 왜 셸 별칭이 아니라 바이너리인가

셸 별칭은 그 별칭을 정의한 셸에서, 그 셸의 설정 파일을 통해서만 동작한다.
바이너리가 추가 이름을 직접 제공하면 셸 종류와 무관하게 동작하고 사용자의
셸 설정 파일을 건드리지 않는다. 같은 모노레포의 `jg` 와 `jgw` 가 이미 이
방식을 쓰고 있어 검증된 패턴이다.

## 구현 방식 — multi-call 바이너리

`gtidy` 와 `gtidy!` 는 `git-tidy` 바이너리를 가리키는 심볼릭 링크다. 바이너리의
`main()` 은 자신이 어떤 이름으로 불렸는지를 `filepath.Base(os.Args[0])` 로
확인한다. 이름이 `gtidy!` 이면 인자 목록 앞에 `--run` 을 끼워 넣고, 그 외의
이름이면 인자를 그대로 둔다.

이름 판별과 인자 변형은 `effectiveArgs(invoked string, args []string) []string`
같은 순수 함수로 분리한다. 입출력이 결정적이라 단위 테스트로 검증할 수 있고,
git 호출이나 터미널 입출력에 의존하지 않는다.

## 심볼릭 링크 설치

- `apps/git-tidy/.mise.toml` 의 install 태스크는 `git-tidy` 를 빌드한 뒤 같은
  디렉터리에 `gtidy` 와 `gtidy!` 심볼릭 링크를 만든다. uninstall 태스크는 세
  이름을 모두 지운다.
- `homebrew-tap` 저장소의 `Formula/git-tidy.rb` install 단계는 `git-tidy` 를
  설치한 뒤 `bin.install_symlink` 로 `gtidy` 와 `gtidy!` 를 만든다.
- GitHub Release 아카이브에는 `git-tidy` 바이너리만 담는다. 심볼릭 링크는 설치
  시점에 만들어지므로 아카이브에 별도 항목을 두지 않는다.

## 자동완성

`apps/git-tidy/completions/` 의 zsh·bash 자동완성 파일 하나가 세 이름을 모두
커버한다. zsh 는 `#compdef` 줄에 `git-tidy gtidy gtidy!` 를 함께 적고, bash 는
`complete` 명령에 세 이름을 함께 등록한다. 세 이름은 같은 플래그 집합을
받으므로 완성 후보도 동일하다.

`gtidy!` 의 `!` 가 zsh `#compdef` 지시자에서 이름으로 받아들여지지 않으면
`gtidy!` 는 자동완성 대상에서 빠질 수 있다. 이 경우에도 `git-tidy` 와 `gtidy`
의 자동완성은 동일하게 동작한다.

## 검증

- `effectiveArgs` 순수 함수의 단위 테스트로 이름별 인자 변형을 검증한다.
  `gtidy!` 는 `--run` 이 앞에 붙고 나머지 인자가 보존되는지, `gtidy` 와
  `git-tidy` 는 인자가 그대로인지 확인한다.
- 빌드된 `git-tidy` 바이너리를 `gtidy!` 이름의 심볼릭 링크로 만든 뒤 임시 git
  저장소에서 한 번 실행해, `--run` 경로가 실제로 타지는지 1회성 end-to-end 로
  확인한다.

## 문서 갱신

- `docs/plans/2026-05-22-git-tidy-cleanup-model.md` 의 명령 설명에서 셸 별칭을
  제공하지 않는다는 문장을 삭제하고, `gtidy` 와 `gtidy!` 를 multi-call 단축
  명령으로 제공한다는 설명으로 바꾼다.
- `apps/git-tidy/PRD.md` 의 기능 범위에 `gtidy` 와 `gtidy!` 를 추가한다.
- `apps/git-tidy/README.md` 의 사용법에 두 단축 명령을 적는다.

## 범위 밖

- `go install` 로만 설치한 사용자에게는 심볼릭 링크가 만들어지지 않는다.
  `git-tidy` 명령 자체는 동작하며, 단축 이름이 필요하면 사용자가 직접 링크하거나
  Homebrew·mise 설치 경로를 쓴다.
- 단축 이름을 더 늘리는 것은 이 설계의 범위가 아니다.
