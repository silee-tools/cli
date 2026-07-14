# oma

`oma`는 사람과 에이전트가 같은 절차로 작업 환경을 준비하도록 돕는 CLI다. `oma prep`은 Jira 티켓, 로컬 작업 설명, 날짜 기반 빈 작업 중 하나를 받아 승인 가능한 계획을 만들고, 승인 뒤 Git worktree와 브랜치, 선택한 submodule, 원격 브랜치, Jira 작업 시작 상태를 순서대로 준비한다.

## 설치와 개발

소스 디렉터리에서 개발 빌드를 설치한다.

```bash
cd apps/oma
mise run install
```

개발 설치는 실행 파일을 `$HOME/.local/bin/oma`에 놓고 활성 채널을 `dev`로 바꾼다.

## 작업 입력

`oma prep --help`의 사용법은 다음과 같다.

```text
Usage: oma prep <JIRA-KEY>
       oma prep --description <text>
       oma prep --empty
```

세 입력은 서로 배타적이다.

```bash
oma prep ABC-123
oma prep --description "로컬 문서 정리"
oma prep --empty
```

- Jira 키는 대문자로 정규화하고 Jira 이슈의 제목과 설명을 로컬 스냅샷에 저장한다. 승인된 적용에서는 필요한 담당자, 시작일, Product type, 진행 상태를 채운다.
- `--description`은 Jira를 조회하지 않고 설명을 브랜치와 worktree 이름의 제목으로 사용한다.
- `--empty`는 Jira를 조회하지 않고 로컬 날짜의 `temp-YYYY-MM-DD`를 제목으로 사용한다. 같은 날짜의 동일한 빈 작업은 같은 브랜치와 worktree를 재사용한다.

한글·영문·숫자는 브랜치 이름에 유지되며 기본 브랜치 type은 `feature`다. `--repo`를 생략하면 현재 경로가 속한 Git 저장소를 사용하고, 저장소 밖에서는 실패한다.

## 옵션

| 옵션 | 의미 |
| --- | --- |
| `--description <text>` | Jira 대신 로컬 작업 설명을 사용한다. |
| `--empty` | 날짜 기반 빈 작업을 준비한다. |
| `--repo <path>` | 대상 Git 저장소를 지정한다. |
| `--type <type>` | 브랜치 type을 지정한다. 기본값은 `feature`다. |
| `--base <branch>` | 기준 브랜치를 지정한다. 비대화형 계획에서는 필수다. |
| `--worktree <mode-or-path>` | `new`, `current`, 등록된 기존 worktree 경로 중 하나를 지정한다. 기본값은 `new`다. |
| `--submodule <path>` | 부모와 같은 이름의 브랜치를 만들 submodule을 지정한다. 반복할 수 있다. |
| `--setup-arg <value>` | `scripts/setup-worktree.sh`에 전달할 인자를 추가한다. 반복할 수 있다. |
| `--product-type <config-key>` | 로컬 설정의 Product type 키를 지정한다. |
| `--transition-id <id>` | 자동으로 하나를 고를 수 없는 Jira 전환을 지정한다. |
| `--no-push` | 부모와 submodule의 원격 브랜치 생성을 생략한다. |
| `--dry-run` | 외부 Git·Jira 쓰기 없이 승인 계획과 token을 만든다. |
| `--plan <token>` | 저장된 승인 계획을 적용한다. 새 작업 옵션과 함께 쓸 수 없다. |
| `--yes` | 비대화형 적용을 승인한다. |
| `--json` | stdout에 JSON 문서 하나를 출력하고 진행 메시지는 stderr로 보낸다. |

## 계획과 적용

터미널에서 입력값이 부족하면 입력 종류와 기준 브랜치를 묻는다. `--dry-run` 없이 대화형으로 실행하면 계획을 먼저 보여주고 승인을 받은 뒤 같은 계획을 즉시 적용한다.

비대화형 환경에서는 계획과 적용을 두 호출로 분리한다. 계획할 때 작업 입력과 `--base`를 지정하고, 적용할 때 계획 token과 `--yes`를 함께 전달한다.

```bash
oma prep --description "인증 문서 정리" --base main --dry-run --json > plan.json
token="$(jq -r .plan_token plan.json)"
oma prep --plan "$token" --yes --json
```

계획 token은 30분 동안 유효하며 한 번만 적용할 수 있다. 적용 전에 저장소, 기준 SHA, 브랜치, worktree, submodule, 셋업 스크립트, Jira 상태를 다시 확인한다. 상태가 달라졌거나 token이 만료됐으면 외부 변경을 시작하지 않고 현재 상태의 새 계획과 token을 반환한다.

`--dry-run`은 worktree, 브랜치, 원격 브랜치와 Jira 필드를 바꾸지 않지만 최신 계획을 만들기 위한 로컬 운영 상태는 갱신한다. 원격이 있으면 `git fetch origin`을 실행하고, Jira 입력이면 이슈를 조회해 스냅샷을 교체하며, 승인 대기 계획 파일을 만든다. `--json`은 출력 형식만 바꾸며 승인을 뜻하지 않는다.

## 적용 순서와 재실행

적용은 worktree와 부모 브랜치 생성, 선택한 submodule 준비, `scripts/setup-worktree.sh`, 부모와 submodule push, Jira 작업 시작 순서로 진행한다. 저장소 루트에 셋업 스크립트가 있을 때만 새 worktree 안에서 실행한다. 원격이 있는 경우 모든 push가 성공하기 전에는 Jira를 변경하지 않는다.

결과의 `status`는 다음 의미를 가진다.

| 상태 | 의미 |
| --- | --- |
| `planned` | 적용 전 계획이 준비됐거나 추가 입력이 필요하다. |
| `completed` | 모든 단계가 끝났거나 이미 같은 상태여서 안전하게 재사용했다. |
| `partial` | 일부 외부 변경 뒤 후속 단계가 실패했다. 종료 코드는 0이 아니다. |
| `failed` | 계획을 적용할 수 없다. 종료 코드는 0이 아니다. |

성공한 외부 변경은 실패 시 자동으로 되돌리지 않는다. `partial`이면 동일한 작업 입력으로 새 계획을 만든 뒤 새 token으로 적용한다. 기존 worktree와 같은 커밋의 원격 브랜치는 재사용하며, 원격이 앞서 있거나 갈라졌으면 force push하지 않는다.

셋업이 성공하면 setup receipt를 상태 디렉터리에 영구적으로 기록한다. 이후 push나 Jira 단계가 실패해도 동일한 저장소·worktree·브랜치·기준 SHA·셋업 스크립트 해시·인자·submodule 조합의 재실행은 셋업을 다시 수행하지 않는다. setup receipt 자체에는 expiry가 없으며 자동으로 제거하지 않는다. 셋업 입력이나 스크립트가 바뀌면 다른 식별자가 만들어져 새 셋업을 수행한다.

## Jira 설정과 XDG 경로

Jira 인증은 `$HOME/.netrc`에서 읽고 설정·계획·출력에 복제하지 않는다. 설정 파일에는 Jira 주소와 필드 좌표만 둔다.

```toml
jira_base_url = "https://jira.example.com"
default_project = "ABC"
product_type_field = "customfield_10001"
start_date_field = "customfield_10002"

[product_type_options]
feature = "Feature"
maintenance = "Maintenance"
```

```text
machine jira.example.com
login user@example.com
password API_TOKEN
```

| 데이터 | XDG 경로 | 기본 경로 |
| --- | --- | --- |
| 설정 | `$XDG_CONFIG_HOME/oma/config.toml` | `$HOME/.config/oma/config.toml` |
| 호환 설정 | `$XDG_CONFIG_HOME/prep-task/config.toml` | `$HOME/.config/prep-task/config.toml` |
| Jira 스냅샷 | `$XDG_CACHE_HOME/oma/jira/<host>/<KEY>.json` | `$HOME/.cache/oma/jira/<host>/<KEY>.json` |
| 계획과 setup receipt | `$XDG_STATE_HOME/oma/` | `$HOME/.local/state/oma/` |

정본 설정이 없고 호환 경로에 일반 파일이 있으면 Jira 계획은 기존 파일을 읽고 설정 migration과 설정 파일 상태 지문을 계획에 포함한다. 계획과 적용 사이에 설정이나 중단된 migration 상태가 달라지면 외부 변경을 시작하지 않고 새 계획과 token을 반환한다. 승인된 Jira 적용의 첫 단계에서 내용을 검증해 정본을 원자적으로 만들고, 호환 경로를 정본의 심볼릭 링크로 바꾼다. migration이 실패하면 Git과 Jira 변경을 시작하지 않으며 기존 파일을 복구한다. 작업 설명과 빈 작업은 Jira 설정을 읽거나 migration하지 않는다.

Jira 스냅샷은 TTL 캐시가 아니다. Jira 작업을 계획할 때 최신 이슈를 다시 조회해 원자적으로 교체하고, Jira 변경 뒤 최종 상태를 다시 저장한다. 새 worktree와 에이전트는 이 파일에서 작업 문맥을 이어받을 수 있다.

## 자동완성

zsh와 Bash completion은 `prep` 옵션, 로컬 Git의 기준 브랜치·등록된 worktree·submodule, 브랜치 type, 로컬 설정의 Product type 키를 제안한다. 후보 조회는 로컬 파일과 Git 메타데이터만 읽으며 Jira나 다른 네트워크 API를 호출하지 않는다.

```bash
# zsh: completions 디렉터리를 fpath에 추가한 뒤 compinit을 실행한다.
fpath=("$PWD/completions" $fpath)
autoload -Uz compinit && compinit

# Bash
source completions/oma.bash
```

## 검증

```bash
mise run completion-check
mise run fmt-check
mise run lint
mise run test
mise run build
```
