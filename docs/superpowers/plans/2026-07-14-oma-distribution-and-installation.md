# oma 배포 준비와 설치 배선 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use `oh-my-agents:adversarial-sdd` as the entry point; it invokes `superpowers:subagent-driven-development` task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 검증을 마친 `oma`의 첫 릴리스 전에 Homebrew Formula를 준비하고, 사용자가 릴리스를 승인한 뒤 실제 artifact가 Formula에 반영됐음을 확인한 다음 두 Mac 장비 프로필에 설치·검증 배선을 추가한다.

**Architecture:** `silee-tools/homebrew-tap`은 prebuilt artifact와 runtime channel 전환을 소유하고, 장비 청사진 저장소는 package 설치와 장비별 활성화만 소유한다. Formula 골격은 CLI 릴리스보다 먼저 merge되어 자동 SHA 갱신 대상이 되고, 실제 publish·release는 사용자 승인 경계에서 멈춘다. 장비 배선은 Formula의 실제 설치 검증이 끝난 뒤 별도 worktree에서 진행한다.

**Tech Stack:** Homebrew Ruby Formula DSL, POSIX sh, YAML device profiles, mise, 기존 `silee-tools/cli` release-please와 GoReleaser artifact.

## 전역 제약과 순서

- 실행 순서는 CLI 구현·검증 완료 → Formula 골격 merge → CLI 구현 merge → 사용자 릴리스 → 자동 Formula version/SHA 갱신 확인 → Homebrew 설치 smoke → oh-my-mbp 배선이다.
- 에이전트는 release, publish, tag 생성, 장비 bootstrap을 자동 실행하지 않는다. 각 외부 상태 변경 전에 사용자에게 현재 상태와 실행 명령을 보여주고 별도 승인을 받는다.
- 각 저장소는 `superpowers:using-git-worktrees`로 별도 worktree를 만든다. `oh-my-mbp`의 기존 checkout은 원격보다 뒤처졌고 사용자 변경이 있으므로 수정하지 않는다.
- 파일 생성 전 저장소별 before/after 트리와 변경 라벨을 제시한다.
- Formula의 64자리 0 SHA는 첫 릴리스 전 골격에서만 허용한다. 실제 설치 완료 판정에는 사용할 수 없다.
- `brew audit`의 경로 인자는 Homebrew 6에서 금지된다. merge 전에는 `brew style Formula/oma.rb`, tap 반영 후에는 `brew audit --strict silee-tools/tap/oma`를 사용한다.

### Task 1: homebrew-tap 기준 상태와 Formula 계약 Red를 확인한다

**Repository:** `silee-tools/homebrew-tap`

**Files:**

- Create: `Formula/oma.rb`
- Modify: `README.md`

**Interfaces:** Formula class는 `Oma`, binary는 `oma`, 초기 version은 `0.1.0`, artifact는 darwin/linux의 amd64/arm64 네 종류다. `post_install`은 `var/silee-tools/oma/active-channel`을 원자 교체해 `channel=release`를 기록한다.

- [ ] `git fetch origin && git status -sb && git log @{u}.. --oneline`으로 기준을 맞추고 clean 상태를 확인한다.
- [ ] `mise run lint`를 실행해 기존 Formula와 저장소 게이트의 기준 결과를 기록한다.
- [ ] `Formula/oma.rb`의 최소 골격을 만들되 `post_install`을 넣지 않은 상태에서 `sh scripts/check-runtime-channel-contract.sh`를 실행한다.
- [ ] `Formula/oma.rb: missing def post_install`과 nonzero exit를 Red 증거로 기록한다. 파이프로 exit code를 가리지 않는다.

### Task 2: Formula 골격과 문서를 Green으로 만든다

**Repository:** `silee-tools/homebrew-tap`

**Files:**

- Modify: `Formula/oma.rb`
- Modify: `README.md`

**Interfaces:** `install`은 `bin.install "oma"`, `zsh_completion.install "completions/_oma"`, `bash_completion.install "completions/oma.bash" => "oma"`를 수행한다. `test do`는 `oma --help`와 `oma prep --help`를 모두 확인한다.

- [ ] 기존 Formula의 URL 계약에 맞춰 `oma/v#{version}/oma-v#{version}-<os>-<arch>.tar.gz` 네 URL과 64자리 0 SHA를 작성한다.
- [ ] `post_install`은 같은 디렉터리 임시 파일을 `rename`하여 release channel을 원자적으로 전환한다.
- [ ] README Usage와 Formula 표에 설치 명령과 `oma` 설명을 추가한다.
- [ ] `ruby -c Formula/oma.rb`, `brew style Formula/oma.rb`, `sh scripts/check-runtime-channel-contract.sh`, `mise run lint`, `git diff --check`를 실행한다.
- [ ] Formula와 README만 diff에 포함됐는지 확인하고 `feat(oma): add formula skeleton`로 커밋한다.
- [ ] PR 본문에 새 workflow 대신 기존 `formula-syntax.yml`의 `Formula/**/*.rb` glob과 runtime contract job에 흡수했다는 선택 근거를 적는다.

### Task 3: 사용자 릴리스 경계에서 멈추고 자동 갱신을 검증한다

**Repositories:** `silee-tools/cli`, `silee-tools/homebrew-tap`

**Files:** 이 작업은 읽기 전용 상태 검증이며 파일을 직접 수정하지 않는다.

- [ ] Formula PR이 main에 반영되고 `Formula/oma.rb`가 실제로 존재하는지 확인한다.
- [ ] CLI 구현 PR의 checks, review decision, merge state와 release-please 설정을 확인한다.
- [ ] 사용자에게 release-please Release PR merge가 tag·GitHub Release·artifact upload·Formula 자동 commit을 일으킨다는 외부 부수효과를 설명하고 진행 여부를 묻는다.
- [ ] 사용자가 직접 Release PR을 merge하거나 명시적으로 별도 실행을 요청하기 전에는 tag, release, publish 명령을 실행하지 않는다.
- [ ] 사용자가 릴리스 완료를 알리면 먼저 `git fetch`로 두 저장소 상태를 다시 맞추고 GitHub Release에 네 archive와 checksums가 존재하는지 읽기 전용으로 확인한다.
- [ ] `Formula/oma.rb`의 version이 실제 tag와 같고 네 SHA가 0이 아니며 download artifact의 SHA-256과 일치하는지 확인한다.

### Task 4: 실제 Homebrew 설치 경로를 검증한다

**Repository:** `silee-tools/homebrew-tap`

**Files:** 이 작업은 로컬 패키지 상태를 변경하므로 실행 직전 사용자 승인이 필요하다.

- [ ] 사용자 승인 후 `brew update`와 `brew install silee-tools/tap/oma` 또는 이미 설치된 경우 `brew upgrade silee-tools/tap/oma`를 실행한다.
- [ ] `brew test silee-tools/tap/oma`, `brew audit --strict silee-tools/tap/oma`, `oma --version`, `oma prep --help`를 실행한다.
- [ ] `oma --version`이 표준 한 줄이고 completion 파일과 release binary가 Homebrew prefix에 설치됐는지 확인한다.
- [ ] active-channel 파일이 `channel=release`이며 일반 사용자 소유로 읽을 수 있는지 확인한다.
- [ ] 하나라도 실패하면 oh-my-mbp 배선을 시작하지 않고 Formula·release artifact 중 어느 경계가 원인인지 진단한다.

### Task 5: oh-my-mbp 설치 계약 테스트를 Red로 만든다

**Repository:** `oh-my-mbp`

**Files:**

- Modify: `setup/tests/profile-features-test.sh`
- Modify: `setup/tests/feature-setup-test.sh`

**Interfaces:** `features/oma/setup.yaml`은 brew package `silee-tools/tap/oma`와 trust `silee-tools/tap`을 선언한다. `check-profile.sh`는 기존 package feature와 같은 6필드 TSV 한 행을 출력한다.

- [ ] 원 checkout의 사용자 변경을 보존하고, fetch한 origin/main에서 별도 worktree를 만든다.
- [ ] `macbook_common_features`에 `oma`를 추가하는 profile test와 `assert_setup_has_name "$ROOT/features/oma/setup.yaml" silee-tools/tap/oma` 계약을 먼저 작성한다.
- [ ] `sh setup/tests/profile-features-test.sh`와 `sh setup/tests/feature-setup-test.sh`를 각각 실행해 profile 또는 feature 부재 assertion Red를 확인한다.

### Task 6: oma feature와 두 장비 프로필을 Green으로 만든다

**Repository:** `oh-my-mbp`

**Files:**

- Create: `features/oma/setup.yaml`
- Create: `features/oma/check-profile.sh`
- Modify: Task 5에서 실제 profile 데이터로 확정한 두 Mac device profile

- [ ] `features/oma/setup.yaml`에 package와 tap trust를 선언한다. Jira 설정이나 자격은 이 저장소에 추가하지 않는다.
- [ ] POSIX `#!/bin/sh` check script가 `installed`, `oma`, `profile`, `no-runtime-check`를 포함한 6필드 TSV를 출력하게 한다.
- [ ] `yq`로 실제 두 Mac profile의 feature 목록을 읽고, 기존 `jg` 바로 다음에 `oma`를 추가한다. 경로와 hostname은 커밋 메시지·PR 본문·완료 보고에 복제하지 않는다.
- [ ] targeted Green으로 두 Red 테스트, `sh setup/tests/trust-test.sh`, `shellcheck features/oma/check-profile.sh`를 실행한다.
- [ ] 전체 Green으로 `MISE_TRUSTED_CONFIG_PATHS="$PWD/.mise.toml" mise run test`와 같은 환경의 `mise run lint`를 실행한다.
- [ ] `git diff --check`, PII·시크릿 검사, Conventional Commit 검증 뒤 `feat(oma): wire CLI installation`으로 커밋한다.

### Task 7: 장비 적용은 별도 승인 뒤 검증한다

**Repository:** `oh-my-mbp`

**Files:** 실제 장비 상태만 바뀌며 저장소 파일 변경은 없다.

- [ ] 각 대상 장비의 `devices/<host>/README.md`가 있으면 읽고 직접 접근 채널과 bootstrap 절차를 확인한다.
- [ ] 적용 전 `HOST_OVERRIDE=<device> sh setup/check-installed.sh --format tsv`로 `oma`가 missing인지 확인한다.
- [ ] 사용자에게 대상 장비와 bootstrap의 변경 범위를 보여주고 별도 승인을 받는다. 승인 전에는 적용하지 않는다.
- [ ] 승인된 장비에서 정본 bootstrap 경로를 실행하고, 같은 check 명령이 installed를 보고하는지 확인한다.
- [ ] 각 장비에서 `oma --version`, `oma prep --help`를 실행한다.
- [ ] 사용자 지정 티켓·저장소·기준 ref에 한해 `oma prep "$OMA_SMOKE_ISSUE_KEY" --repo "$OMA_SMOKE_REPO" --base "$OMA_SMOKE_BASE" --dry-run --json`을 실행한다.
- [ ] dry-run에서는 옛 prep-task config 일반 파일이 그대로이고 새 config·symlink가 생기지 않았음을 확인한다. snapshot·plan은 `0600`, 부모는 `0700`인지 확인하고 Jira·Git 외부 상태가 바뀌지 않았음을 직접 조회한다.
- [ ] config 정본 생성과 legacy symlink 마이그레이션은 사용자가 최초 적용을 승인한 실행에서만 검증하며, 그 전에는 완료 조건으로 요구하지 않는다.
- [ ] 두 장비 설치와 적어도 한 장비의 실제 dry-run이 통과한 뒤에만 `oh-my-agents` 통합 계획으로 넘어간다.
