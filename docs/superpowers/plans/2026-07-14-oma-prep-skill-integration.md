# prep-task 스킬의 oma 통합 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use `oh-my-agents:adversarial-sdd` as the entry point; it invokes `superpowers:subagent-driven-development` task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 실제 장비에 설치·검증된 `oma`를 `prep-task`의 유일한 실행 계층으로 사용하고, 스킬은 인터뷰·계획 설명·승인·세션 이동·Jira 문맥 요약만 맡게 한다.

**Architecture:** `plugin/skills/prep-task/SKILL.md`는 입력을 `oma prep` 옵션으로 변환해 먼저 `--dry-run --json`을 실행하고, 구조화된 `required_inputs`를 사용자에게 묻고 새 계획을 만든다. 명시적 승인 뒤 같은 옵션과 `--plan <token> --yes --json`으로 적용하며, `completed`이고 반환 worktree가 현재 위치와 다를 때만 `EnterWorktree`를 호출한다. 기존 Jira REST 레퍼런스와 Git 직접 조작은 제거해 정본을 CLI 하나로 통일한다.

**Tech Stack:** Markdown skill contract, POSIX sh prompt-contract test, Claude/Codex plugin manifests, `scripts/generate-codex-manifests.zsh`.

## 전역 제약

- 이 계획은 배포 계획의 체크포인트대로 두 대상 장비에서 `oma` 설치·help·version 검증을 마치고, 적어도 한 장비에서 실제 Jira `oma prep --dry-run --json` 검증이 완료된 뒤에만 실행한다.
- plugin cache는 읽기 전용이다. 실제 소스 저장소의 새 worktree만 수정한다.
- 기존 Jira 없는 작업 설명과 빈 브랜치를 제거하지 않는다. 각각 `--description`과 `--empty`로 CLI에 위임한다.
- 프롬프트 계약 테스트는 있어야 할 정의부 문장의 존재만 검증한다. 제거된 옛 문구와 파일 참조는 별도 정적 검색으로 확인한다.
- Codex manifest는 직접 수정하지 않고 Claude manifest 두 개를 바꾼 뒤 생성기로 만든다.
- 실제 Jira write와 Git branch 생성은 프롬프트 계약 검증에서 실행하지 않는다. 라이브 smoke는 dry-run만 사용한다.

### Task 1: 현재 계약과 설치 선결 조건을 확인한다

**Repository:** `oh-my-agents`

**Files:** 읽기 전용 선결 확인이므로 수정하지 않는다.

- [ ] `command -v oma`, `oma --version`, `oma prep --help`를 실행해 설치 선결 조건을 확인한다. 실패하면 스킬을 수정하지 않고 배포·설치 계획으로 돌아간다.
- [ ] 두 대상 장비의 설치·help·version 증거와 적어도 한 장비의 실제 Jira dry-run 증거가 모두 있는지 확인한다. 하나라도 없으면 Task 2로 진행하지 않는다.
- [ ] 새 worktree를 만든 직후 `scripts/setup-repo-hooks.sh`를 실행하고 local `core.hooksPath`가 `.githooks`인지 확인한다.
- [ ] 세션 전용 `XDG_CACHE_HOME`과 `XDG_STATE_HOME`만 격리하고, `XDG_CONFIG_HOME`과 `~/.netrc`는 실제 정본을 읽기 전용으로 사용해 사용자 지정 실제 티켓·저장소의 `--dry-run --json`을 실행한다. `issue`, `jira_snapshot_path`, `required_inputs`, token, worktree path 계약을 실제 데이터로 확인한다.
- [ ] dry-run 뒤 Jira fields와 Git refs가 바뀌지 않았음을 직접 확인한다.
- [ ] 저장소의 기존 prep-task 관련 테스트가 없음을 확인하고 전체 baseline 검증을 기록한다.

### Task 2: 새 프롬프트 계약 테스트를 Red로 만든다

**Repository:** `oh-my-agents`

**Files:**

- Create: `plugin/tests/prep-task/verify.sh`

**Interfaces:** verify script는 정의부에 정확히 한 번 있어야 하는 고유 문장을 `grep -Fo`와 count로 검사한다. 스크립트는 POSIX sh와 `set -u`를 사용한다.

- [ ] 다음 계약을 각각 한 번 요구하는 `has_once`와 `assert_contract`를 작성한다.

```text
command -v oma
기존 Jira·Git 절차로 우회하지 않는다
--description
--empty
--dry-run --json
--json은 출력 형식일 뿐 승인으로 해석하지 않는다
required_inputs
--product-type
--transition-id
plan_token과 expires_at을 포함한 변경 미리보기
--plan <token> --yes --json
partial 또는 failed이면 세션을 이동하지 않는다
EnterWorktree(path=...)
기존 worktree는 제거하지 않는다
사용자의 다음 지시를 기다린다
```

- [ ] `sh -n plugin/tests/prep-task/verify.sh`로 구문을 확인한다.
- [ ] `sh plugin/tests/prep-task/verify.sh`를 실행해 새 정의 문장이 없는 assertion Red와 nonzero exit를 확인한다.
- [ ] 테스트 파일만 추가한 상태를 커밋하지 않고 Green 작업으로 이어간다.

### Task 3: SKILL.md를 oma 호출 계층으로 교체한다

**Repository:** `oh-my-agents`

**Files:**

- Modify: `plugin/skills/prep-task/SKILL.md`

**Interfaces:** frontmatter는 `AskUserQuestion, Bash, EnterWorktree`만 허용한다. Jira·Git 직접 실행과 Task 위임은 사용하지 않는다.

- [ ] 입력 인터뷰에서 Jira 키·URL, 작업 설명, 빈 브랜치를 구분하고 각각 positional key, `--description`, `--empty`로 변환한다.
- [ ] repo, base, worktree, submodule, setup args, branch type, push 정책의 사용자 선택을 CLI 옵션으로만 전달한다. 저장소 상태를 스킬이 별도로 Git 명령으로 재계산하지 않는다.
- [ ] 첫 호출은 `oma prep ... --dry-run --json`이며 `--json`을 승인으로 해석하지 않는다고 정의한다.
- [ ] `required_inputs`가 있으면 Product type과 transition 후보를 실제 JSON label로 보여주고, 답을 각각 `--product-type` 또는 `--transition-id`에 반영해 새 dry-run을 만든다. 두 옵션과 매핑 문장은 고유 프롬프트 계약 anchor로 검증한다.
- [ ] `plan_token`, `expires_at`, branch, base, worktree, steps, warnings와 dry-run의 로컬 상태 변경을 사용자에게 보여주고 명시적 승인을 받는다.
- [ ] 적용 호출은 계획 때 사용한 작업 옵션을 유지한 `oma prep ... --plan <token> --yes --json`이다. 만료·drift로 새 계획이 반환되면 다시 승인받는다.
- [ ] `partial`·`failed`이면 세션을 이동하지 않고 완료 단계·실패 단계·재실행 방법을 보고한다.
- [ ] `completed`이고 반환 worktree 절대경로가 현재 위치와 다를 때만 `EnterWorktree(path=...)`를 호출한다. `current` 모드는 이동하지 않고, 기존 worktree 모드는 반환 경로로 이동하며 어떤 worktree도 제거하지 않는다.
- [ ] 완료 뒤 `issue.summary`, `issue.description_text`, 상태, snapshot 경로를 요약하고 계획·구현을 시작하지 않은 채 사용자의 다음 지시를 기다린다. non-Jira 입력은 description 또는 empty 문맥을 요약한다.
- [ ] `sh plugin/tests/prep-task/verify.sh`를 실행해 Green을 확인하고 각 anchor가 본문에 그대로 한 번 존재하는지 `grep -Fq`로 교차 확인한다.

### Task 4: 이중 정본을 제거하고 참조를 전수 정리한다

**Repository:** `oh-my-agents`

**Files:**

- Delete: `plugin/skills/prep-task/references/jira-rest-reference.md`
- Modify: `README.md`

- [ ] `SKILL.md`가 더 이상 reference를 소비하지 않는 Green 상태에서 Jira REST reference를 삭제한다.
- [ ] README의 검증 명령 목록에 `sh plugin/tests/prep-task/verify.sh`를 추가한다.
- [ ] `rg -n '(plugin/skills/prep-task/references/)?jira-rest-reference\.md|jira-rest-reference\.md' .`로 prefix가 있는 경로와 없는 이름을 함께 검색해 일치 없음인지 확인한다. 이 부재 검사는 프롬프트 계약 테스트에 넣지 않는다.
- [ ] `rg -n '/tmp/jira_issue_|curl --netrc|git worktree remove|git worktree add' plugin/skills/prep-task README.md`로 제거된 직접 실행 계약이 남지 않았는지 정적 검토한다.
- [ ] 삭제 파일의 모든 참조가 정리됐고 새 계약 테스트는 여전히 Green인지 확인한다.

### Task 5: 플러그인 버전과 생성 manifest를 갱신한다

**Repository:** `oh-my-agents`

**Files:**

- Modify: `plugin/.claude-plugin/plugin.json`
- Modify: `.claude-plugin/marketplace.json`
- Regenerate: `plugin/.codex-plugin/plugin.json`
- Regenerate: `.codex-plugin/marketplace.json`

**Interfaces:** 실행 시 네 manifest의 현재 버전이 같은지 먼저 확인하고, 현재 `MAJOR.MINOR.PATCH`의 다음 minor인 `MAJOR.(MINOR+1).0`으로 네 manifest를 정렬한다. 기준 브랜치보다 반드시 증가해야 한다.

- [ ] 네 manifest의 현재 버전이 다르면 수정하지 않고 실패한다. 같으면 다음 minor를 계산해 Claude manifest 두 개만 수정한다.
- [ ] `scripts/generate-codex-manifests.zsh`를 실행해 Codex manifest 두 개를 생성한다.
- [ ] 생성 파일 해시를 기록하고 생성기를 다시 실행해 해시가 유지되는지 확인한다.
- [ ] 네 JSON을 `python3 -m json.tool`로 검증한다.
- [ ] 변경을 stage한 뒤 `sh plugin/tests/release/check-version-bump.sh`로 네 manifest와 기준 브랜치 대비 증가를 확인한다.

### Task 6: 전체 정적·라이브 dry-run 검증을 완료한다

**Repository:** `oh-my-agents`

**Files:** 위 작업의 전체 변경분.

- [ ] `sh -n plugin/tests/prep-task/verify.sh`와 `sh plugin/tests/prep-task/verify.sh`를 실행한다.
- [ ] 저장소 README에 기록된 전체 plugin 검증과 JSON·YAML·shell syntax 검사를 실행한다.
- [ ] `git diff --check`와 삭제 참조 sweep을 다시 실행한다.
- [ ] 실제 `prep-task` 호출을 Jira, description, empty 세 입력으로 수행하되 적용 승인 직전에서 멈춰 스킬이 세 입력을 올바른 dry-run으로 변환하는지 확인한다.
- [ ] 실제 Jira dry-run 결과의 summary·description·snapshot 경로가 사용자 요약에 나타나고, `required_inputs` 후보가 실제 label로 질문되는지 확인한다.
- [ ] 승인 거절 시 apply가 실행되지 않는지, fake JSON 또는 CLI test harness로 `partial`·`failed`에서 EnterWorktree가 호출되지 않는지 검증한다.
- [ ] cache 디렉터리를 수정하지 않았는지 확인한다.
- [ ] PII·시크릿 검사를 알려진 일치 사례로 교차 검증하고, 커밋 제목을 `feat(prep-task): delegate preparation to oma`로 검증한다.
- [ ] PR 본문에 직접 Jira·Git 로직 제거 이유, 세 입력 정책, plan/apply 승인 흐름, partial·worktree 이동 경계를 Mermaid flowchart로 설명하고 Red·Green 증거를 남긴다.
