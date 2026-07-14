# oma prep CLI 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use `oh-my-agents:adversarial-sdd` as the entry point; it invokes `superpowers:subagent-driven-development` task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `silee-tools/cli`에 Jira 티켓, 작업 설명, 빈 브랜치를 동일한 계획·승인 계약으로 준비하는 `oma prep` Go CLI를 추가한다.

**Architecture:** `cmd/oma`는 인자 해석과 입출력만 맡고, `internal/prep`이 `config`, `jira`, `gitops`, `state` 인터페이스를 조합한다. 계획은 외부 상태를 조회해 30분짜리 단일 사용 토큰으로 저장하고, 적용은 상태 지문을 다시 검증한 뒤 worktree, setup, push, Jira 순서로 수행한다. 각 외부 경계는 독립 패키지와 테스트 서버 또는 임시 Git 저장소로 검증한다.

**Tech Stack:** Go 1.23, 표준 `flag`·`net/http`·`os/exec`, `github.com/pelletier/go-toml/v2` v2.2.4, `github.com/jdx/go-netrc` v1.0.0, `golang.org/x/text` v0.28.0, `golang.org/x/term` v0.34.0, mise, GoReleaser, GitHub Actions.

## 전역 제약

- 기준 설계는 `docs/superpowers/specs/2026-07-14-oma-prep-design.md`다. 계획과 설계가 다르면 설계를 우선하고 차이를 사용자에게 보고한다.
- 구현은 `superpowers:using-git-worktrees`로 만든 별도 worktree에서 수행한다. 파일 생성 전 실제 대상 트리의 before/after와 `(추가)`·`(수정)` 라벨을 보여주고 승인받는다.
- 먼저 기존 루트 게이트를 실행해 기준 상태를 기록한다. 실패하면 통과하는 대조군과 비교해 기존 문제인지 변경 영향인지 구분한다.
- 각 동작은 컴파일 실패가 아닌 실행 assertion 실패로 Red를 관찰한 뒤 Green을 만든다. 완료 보고와 PR 본문에 Red·Green 명령과 결과를 남긴다.
- 자격, 실제 Jira 본문, 사용자 로컬 절대경로를 테스트 픽스처·커밋·로그에 넣지 않는다. 이메일 픽스처는 `agent@example.com`을 사용한다.
- Git과 HTTP 실패를 빈 결과로 바꾸지 않는다. 성공한 외부 변경을 자동 rollback하지 않고 구조화된 `partial` 결과로 남긴다.
- 릴리스, 태그 생성, 배포는 이 계획에서 수행하지 않는다. 릴리스 등록 파일과 릴리스 가능한 산출물까지만 준비한다.

## 고정 공개 타입과 인터페이스

다음 계약은 패키지 사이 기준이다. 구현 중 이름이나 필드를 바꿔야 하면 호출자와 테스트를 같은 작업에서 함께 갱신한다.

```go
// internal/prep/types.go
type InputKind string
const (
    InputJira        InputKind = "jira"
    InputDescription InputKind = "description"
    InputEmpty       InputKind = "empty"
)

type Input struct {
    Kind InputKind
    IssueKey, Description, Repo, BranchType, Base, Worktree string
    ProductType, TransitionID string
    Submodules, SetupArgs []string
    NoPush bool
}

type RequiredInput struct {
    Kind, Message string
    Options []InputOption
}

type InputOption struct { Value, Label string }
type Base struct { Ref, SHA string }
type Step struct { Name, Status, Detail string }
type IssueContext struct {
    Key, Summary, DescriptionText, Status, Assignee string
}

type Result struct {
    Status, PlanToken string
    ExpiresAt time.Time
    InputKind InputKind
    IssueKey string
    Issue *IssueContext
    JiraSnapshotPath, Branch, WorktreePath, NextAction string
    Base Base
    Steps []Step
    Warnings []string
    RequiredInputs []RequiredInput
}
```

```go
// internal/prep/planner.go
type JiraGateway interface {
    FetchIssue(context.Context, string) (jira.Issue, []byte, error)
    Myself(context.Context) (jira.User, error)
    Transitions(context.Context, string) ([]jira.Transition, error)
    UpdateFields(context.Context, string, map[string]any) error
    ApplyTransition(context.Context, string, string) error
}

type ConfigGateway interface {
    Load() (config.Config, config.Source, error)
    ApplyMigration(validate func(config.Config) error) error
}

type GitGateway interface {
    Inspect(context.Context, gitops.InspectRequest) (gitops.Snapshot, error)
    CreateWorktree(context.Context, gitops.Operation) error
    PrepareSubmodules(context.Context, string, []gitops.SubmoduleOperation) error
    RunSetup(context.Context, string, []string) error
    Push(context.Context, string, string) error
}

func (p *Planner) Plan(context.Context, Input) (Result, error)
func (p *Planner) Apply(context.Context, string) (Result, error)
```

```go
// cmd/oma/main.go
type options struct {
    Input prep.Input
    PlanToken string
    DryRun, Yes, JSON bool
}
func parseOptions(args []string) (options, error)
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, deps dependencies) error
func versionLine(name, version string) string
```

### Task 1: 앱 골격과 런타임 채널을 만든다

**Files:**

- Create: `apps/oma/go.mod`
- Create: `apps/oma/.gitignore`
- Create: `apps/oma/.mise.toml`
- Create: `apps/oma/.goreleaser.yaml`
- Create: `apps/oma/cmd/oma/main.go`
- Create: `apps/oma/cmd/oma/main_test.go`
- Create: `apps/oma/internal/runtimechannel/channel.go`
- Create: `apps/oma/internal/runtimechannel/channel_test.go`

**Interfaces:** `main`은 `runtimechannel.ReleaseExecutable(version, runtimeStatePath, "oma")`를 호출하고 release 채널이면 `syscall.Exec`으로 고정된 Homebrew 바이너리를 실행한다. `-v`와 `--version`은 정확히 `oma v<version> © 2026 silee-tools`를 출력한다.

- [ ] 루트에서 `mise run version-check`, `mise run runtime-channel-check`, `mise run lint`를 실행해 기준 결과를 기록한다.
- [ ] module path가 `github.com/silee-tools/oma`이고 `go 1.23`인 `go.mod`를 만든다. `main_test.go`에 `TestVersionLine`과 `TestRunShowsPrepHelp`를 추가하되 `versionLine`과 `run`의 최소 stub은 잘못된 문자열 또는 오류를 반환하게 한다.

```go
func TestVersionLine(t *testing.T) {
    if got := versionLine("oma", "1.2.3"); got != "oma v1.2.3 © 2026 silee-tools" {
        t.Fatalf("got %q", got)
    }
}
```

- [ ] `cd apps/oma && mise exec -- go test ./cmd/oma -run 'TestVersionLine|TestRunShowsPrepHelp' -count=1`을 실행해 문자열 assertion 실패를 확인한다.
- [ ] 형제 앱의 검증된 runtime channel 계약을 `oma` 이름과 경로로 구현하고 `.mise.toml`의 `install`을 임시 바이너리·백업·원자적 `channel=dev` 전환 방식으로 작성한다.
- [ ] `.goreleaser.yaml`에 `./cmd/oma`, `CGO_ENABLED=0`, darwin/linux, amd64/arm64, `completions/*` archive 포함, `release.disable: true`를 선언한다.
- [ ] `cd apps/oma && mise run fmt-check && mise run test && mise run build`를 실행해 Green을 확인한다.
- [ ] `git diff --check` 후 `feat(oma): scaffold agent workflow CLI`로 커밋한다.

### Task 2: 명령 입력과 이름 정규화를 구현한다

**Files:**

- Modify: `apps/oma/cmd/oma/main.go`
- Modify: `apps/oma/cmd/oma/main_test.go`
- Create: `apps/oma/cmd/oma/prompt.go`
- Create: `apps/oma/cmd/oma/prompt_test.go`
- Create: `apps/oma/internal/prep/types.go`
- Create: `apps/oma/internal/prep/naming.go`
- Create: `apps/oma/internal/prep/naming_test.go`

**Interfaces:** `parseOptions(args []string) (options, error)`, `Prompter.Select(label string, options []promptOption) (string, error)`, `Prompter.Confirm(message string) (bool, error)`, `NormalizeIssueKey(string) (string, error)`, `Slug(string) (string, error)`, `BranchName(InputKind, branchType, key, title string, today time.Time) (string, error)`, `WorktreeName(InputKind, key, title string, today time.Time) (string, error)`를 제공한다. `dependencies`는 주입 가능한 `IsTerminal func() bool`과 `Prompter`를 가진다.

- [ ] Jira 키·`--description`·`--empty` 상호 배타성과 `--repo`, `--type`, `--base`, `--worktree`, 반복 `--submodule`, 반복 `--setup-arg`, `--product-type`, `--transition-id`, `--no-push`, `--dry-run`, `--plan`, `--yes`, `--json`을 검증하는 table test를 먼저 작성한다. `--type`의 기본값은 `feature`이고 비대화형 계획의 `--base`는 필수다.
- [ ] TTY에서는 누락된 input kind와 base를 실제 후보에서 고르고, non-TTY에서는 필요한 flag가 없으면 실패하며, 취소와 최종 승인 거절은 외부 write 없이 끝나는 prompt test를 작성한다.
- [ ] 한글 유지, NFC, emoji 제거, 구분자 평탄화, 50 Unicode 문자, 빈 slug, `feature/ABC-123-제목`, `feature/설명`, `feature/temp-2026-07-14`를 검증한다.

```go
func TestSlugKeepsKorean(t *testing.T) {
    got, err := Slug(" 결제 완료 ✅ / 영수증 ")
    if err != nil || got != "결제-완료-영수증" {
        t.Fatalf("got %q, err %v", got, err)
    }
}
```

- [ ] `cd apps/oma && mise exec -- go test ./internal/prep ./cmd/oma -run 'TestSlugKeepsKorean|TestParseOptions|TestPrompt' -count=1 -v`로 실행 assertion Red를 확인하고 실제 실행 test 수가 0이 아님을 확인한다.
- [ ] `utf8` rune 단위가 아니라 `[]rune`으로 길이를 제한하고, `norm.NFC.String` 뒤 허용 문자와 Git 금지 문자를 판정한다.
- [ ] `git check-ref-format --branch`는 Task 5의 Git 경계에서도 재검증하도록 이름 생성 결과를 구조체로 전달한다.
- [ ] `go get golang.org/x/text@v0.28.0 golang.org/x/term@v0.34.0`과 `go mod tidy`를 실행하고 `go.mod`의 `go 1.23` 유지와 두 모듈의 최소 Go 버전을 확인한다.
- [ ] 같은 테스트를 Green으로 만든다.
- [ ] `feat(oma): define prep command inputs`로 커밋한다.

### Task 3: XDG 경로와 설정 마이그레이션을 구현한다

**Files:**

- Create: `apps/oma/internal/config/paths.go`
- Create: `apps/oma/internal/config/paths_test.go`
- Create: `apps/oma/internal/config/config.go`
- Create: `apps/oma/internal/config/config_test.go`
- Create: `apps/oma/internal/config/migration.go`
- Create: `apps/oma/internal/config/migration_test.go`

**Interfaces:**

```go
type Paths struct { Canonical, Legacy, CacheRoot, StateRoot, Netrc string }
type Source string
type Config struct {
    JiraBaseURL, DefaultProject, ProductTypeField, StartDateField string
    ProductTypeOptions map[string]string
}
func ResolvePaths(getenv func(string) string, home string) Paths
func Load(Paths) (Config, Source, error)
func PlanMigration(Paths) (*Migration, error)
func (m Migration) Apply(validate func(Config) error) error
```

- [ ] XDG 변수별 경로, HOME fallback, canonical 우선, legacy 일반 파일 읽기, legacy symlink, 둘 다 다른 일반 파일인 충돌을 table test로 작성한다.
- [ ] 마이그레이션 성공 시 canonical `0600`, 부모 `0700`, legacy symlink가 되고, TOML·인증 검증 실패와 symlink 교체 실패 시 legacy 원본이 복원되는 테스트를 작성한다.
- [ ] `cd apps/oma && mise exec -- go test ./internal/config -run 'TestResolvePaths|TestMigration' -count=1 -v`로 Red를 확인한다.
- [ ] `go get github.com/pelletier/go-toml/v2@v2.2.4`를 실행한 뒤 TOML import를 추가하고 `go mod tidy`로 정리한다.
- [ ] 임시 파일을 같은 디렉터리에 만들고 `chmod` 후 `rename`한다. 충돌 파일은 자동 덮어쓰지 않으며 rollback 중 발생한 오류도 원래 오류와 함께 반환한다.
- [ ] Green 뒤 `go test -race ./internal/config`로 원자 교체 경로를 검증한다.
- [ ] `feat(oma): migrate prep configuration`으로 커밋한다.

### Task 4: Jira 조회·스냅샷·전환 정책을 구현한다

**Files:**

- Create: `apps/oma/internal/jira/types.go`
- Create: `apps/oma/internal/jira/client.go`
- Create: `apps/oma/internal/jira/client_test.go`
- Create: `apps/oma/internal/jira/transitions.go`
- Create: `apps/oma/internal/jira/transitions_test.go`
- Create: `apps/oma/internal/jira/snapshot.go`
- Create: `apps/oma/internal/jira/snapshot_test.go`

**Interfaces:** `CredentialsFromNetrc(path, host string) (Credentials, error)`, `NewClient(baseURL string, httpClient *http.Client, credentials Credentials) (*Client, error)`, `FetchIssue`, `Myself`, `Transitions`, `UpdateFields`, `ApplyTransition`, `SelectTransition(current Status, available []Transition, requestedID string) (TransitionDecision, error)`, `WriteSnapshot(path string, raw []byte) error`를 제공한다.

- [ ] `httptest.Server`로 전체 필드 GET, ADF 설명의 plain text 변환, current user, 바꿀 필드만 포함한 PUT, transition POST를 먼저 테스트한다.
- [ ] 직접 indeterminate 하나, new 중간 후보 하나, 복수 후보의 `required_inputs`, 유효·무효 `--transition-id`, done 차단, 3-hop 상한을 table test로 작성한다.
- [ ] non-2xx, 잘못된 JSON, 조회 실패가 빈 이슈·빈 전환으로 바뀌지 않고 제한 길이 오류로 전파되며 Authorization과 응답 원문 전체가 오류에 포함되지 않는지 검사한다.
- [ ] `cd apps/oma && mise exec -- go test ./internal/jira -run 'TestClient|TestSelectTransition|TestWriteSnapshot' -count=1 -v`로 Red를 확인한다.
- [ ] `go get github.com/jdx/go-netrc@v1.0.0`을 실행한 뒤 netrc import를 추가하고 `go mod tidy`로 정리한다.
- [ ] 동적 custom field는 고정 필드와 `map[string]json.RawMessage`를 함께 사용하고, 스냅샷은 `0600` 임시 파일을 원자 교체한다.
- [ ] 같은 명령과 `go test -race ./internal/jira`를 Green으로 만든다.
- [ ] `feat(oma): add Jira preparation gateway`로 커밋한다.

### Task 5: Git 조회·worktree·setup·push를 구현한다

**Files:**

- Create: `apps/oma/internal/gitops/runner.go`
- Create: `apps/oma/internal/gitops/runner_test.go`
- Create: `apps/oma/internal/gitops/repository.go`
- Create: `apps/oma/internal/gitops/repository_test.go`
- Create: `apps/oma/internal/gitops/worktree.go`
- Create: `apps/oma/internal/gitops/worktree_test.go`
- Create: `apps/oma/internal/gitops/push.go`
- Create: `apps/oma/internal/gitops/push_test.go`

**Interfaces:**

```go
type Runner interface { Run(context.Context, string, ...string) ([]byte, error) }
type Worktree struct { Path, Branch, Head string }
type Submodule struct { Path, URL, BaseRef, BaseSHA string }
type Operation struct { Repo, Path, Branch, BaseSHA string }
type SubmoduleOperation struct { Path, URL, Branch, BaseRef, BaseSHA string }
type Snapshot struct { RepoRoot, CommonDir, BaseRef, BaseSHA string; Worktrees []Worktree; Submodules []Submodule; SetupHash string }
type InspectRequest struct { Repo, Base, Branch, Worktree string; Submodules, SetupArgs []string; NoPush bool }
func NormalizeRepo(context.Context, Runner, string) (string, string, error)
func FetchOrigin(context.Context, Runner, string) error
func DefaultBase(context.Context, Runner, string) (string, []string, error)
func Inspect(context.Context, Runner, InspectRequest) (Snapshot, error)
func CreateWorktree(context.Context, Runner, Operation) error
func PrepareSubmodules(context.Context, Runner, string, []SubmoduleOperation) error
func RunSetup(context.Context, Runner, string, []string) error
func Push(context.Context, Runner, string, string) error
```

- [ ] 임시 repo와 bare origin helper를 만들고 기본 ref, fetch에 `--prune` 부재, `.worktrees` ignore, porcelain worktree 파싱, dirty current worktree, 동일 branch/path 재사용, 부분 충돌을 테스트한다.
- [ ] `.gitmodules` URL의 remote HEAD를 구조적으로 조회해 선택한 서브모듈 base ref와 SHA를 계획하고, apply에서 선택 경로만 `git submodule update --init -- <path>`한 뒤 같은 브랜치명을 만드는 테스트를 작성한다. 선택하지 않은 서브모듈은 uninitialized 상태를 유지해야 한다.
- [ ] `scripts/setup-worktree.sh`가 없으면 skip, 있으면 새 worktree를 cwd로 `sh scripts/setup-worktree.sh`와 개별 `--setup-arg` argv를 전달하고 실패를 반환하는 테스트를 작성한다. 인자를 생략한 실행도 그대로 수행하며 스크립트가 인자를 요구해 실패하면 worktree를 남기고 push와 Jira 변경 전에 중단한다.
- [ ] 원격 없음, 정상 `push -u origin`, 같은 SHA 재사용, remote ahead/diverged 실패, force 옵션 부재, 부모와 서브모듈 base 독립 탐지를 테스트한다.
- [ ] `cd apps/oma && mise exec -- go test ./internal/gitops -run 'TestCreateWorktreeAndPush|TestRunSetup|TestInspect' -count=1 -v`로 Red를 확인한다.
- [ ] 모든 Git 실행을 `exec.CommandContext("git", args...)`의 분리된 argv로 구현하고 `--porcelain -z`, `for-each-ref --format`, `worktree list --porcelain`을 구조적으로 파싱한다.
- [ ] Green 뒤 실제 실행 기록에서 `push --force`와 `fetch --prune`이 없음을 확인한다.
- [ ] `feat(oma): add Git worktree preparation`으로 커밋한다.

### Task 6: 계획 토큰·상태 지문·동시 사용 차단을 구현한다

**Files:**

- Create: `apps/oma/internal/state/store.go`
- Create: `apps/oma/internal/state/store_test.go`

**Interfaces:** `Store.Create(payload any, fingerprint string)`, `Load`, `Claim`, `Consume`를 제공한다. 토큰은 `crypto/rand` 32바이트 base64url이며 계획은 30분 후 만료된다.

- [ ] 0700 디렉터리, 0600 JSON, 랜덤 토큰 형식, 30분 경계, traversal 거부, 손상 JSON, 단일 사용을 테스트한다.
- [ ] 같은 token을 두 goroutine이 `Claim`할 때 하나만 성공하고 `.json`에서 `.in-use`로 원자 전환되는 테스트를 작성한다.
- [ ] `cd apps/oma && mise exec -- go test ./internal/state -run 'TestStore|TestClaim' -count=1 -race -v`로 Red를 확인한다.
- [ ] 성공·실패와 관계없이 적용 시도 뒤 consumed 상태가 되게 하되, 상태 drift로 적용하지 않고 새 계획을 반환하는 경로도 기존 token을 소비한다.
- [ ] 같은 명령을 Green으로 만든다.
- [ ] `feat(oma): persist approved prep plans`로 커밋한다.

### Task 7: 계획·적용 오케스트레이션과 출력을 연결한다

**Files:**

- Create: `apps/oma/internal/prep/planner.go`
- Create: `apps/oma/internal/prep/planner_test.go`
- Create: `apps/oma/internal/prep/apply.go`
- Create: `apps/oma/internal/prep/apply_test.go`
- Create: `apps/oma/internal/output/render.go`
- Create: `apps/oma/internal/output/render_test.go`
- Modify: `apps/oma/cmd/oma/main.go`
- Modify: `apps/oma/cmd/oma/main_test.go`

**Interfaces:** `Planner.Plan`은 fetch와 Jira 작업의 snapshot을 포함한 조회·로컬 상태 쓰기만 수행한다. `Planner.Apply`는 Jira 입력에서만 config migration을 먼저 수행하고, 모든 입력에서 worktree/부모 branch → 선택 서브모듈 초기화·branch → setup → 부모·서브모듈 push를 수행하며, Jira 입력에서만 fields → 최대 3 transition → final snapshot을 잇는다. `output.Human(io.Writer, prep.Result)`와 `output.JSON(io.Writer, prep.Result)`는 같은 Result를 렌더한다.

- [ ] fake gateway로 Jira/description/empty 세 입력, `required_inputs`, dry-run 무외부-write, 적용 순서, push 실패 전 Jira 무호출, partial 재실행, 상태 drift 새 token을 테스트한다.
- [ ] 비어 있는 HOME/XDG와 호출되면 실패하는 Jira gateway를 사용해 description·empty의 plan/apply가 성공하고 config load·migration·Jira 호출이 모두 0회인지 테스트한다.
- [ ] TTY의 Product type·transition 후보 질문과 최종 승인, non-TTY의 required flag 실패, `--json`이 승인을 대신하지 않는 경로를 `cmd/oma`에서 테스트한다.
- [ ] JSON의 필수 필드, Jira 작업의 `issue`와 `jira_snapshot_path`, non-Jira 작업에서 두 필드 생략, `required_inputs`가 있으면 승인 가능한 token을 생략하는 계약, stdout 단일 JSON, stderr progress 분리를 테스트한다.
- [ ] `cd apps/oma && mise exec -- go test ./internal/prep ./internal/output ./cmd/oma -run 'TestPlan|TestApply|TestJSON|TestRunInteractive' -count=1 -v`로 Red를 확인하고 실제 실행 test 수가 0이 아님을 확인한다.
- [ ] canonical plan payload를 JSON marshal한 뒤 SHA-256으로 fingerprint를 만들고 적용 직전 Jira·Git을 재조회해 비교한다.
- [ ] `partial`과 `failed`는 nonzero exit로 매핑하고, 결과 생성에 실패한 내부 오류도 stderr에 민감정보 없이 기록한다.
- [ ] 같은 명령을 Green으로 만든다.
- [ ] `feat(oma): orchestrate prep planning and apply`로 커밋한다.

### Task 8: 빌드 바이너리 전체 경로를 검증한다

**Files:**

- Create: `apps/oma/tests/prep_e2e_test.go`

**Interfaces:** 테스트는 실제 `oma` 바이너리, 임시 bare origin, 임시 repo, `httptest.Server`, 격리한 XDG/HOME을 사용한다. 실제 Jira나 사용자 설정에 접근하지 않는다.

- [ ] `--dry-run --json`이 fetch·snapshot·plan만 만들고 브랜치·worktree·Jira write는 만들지 않는 실패 테스트를 작성한다.
- [ ] 반환 token으로 `--plan <token> --yes --json`을 실행해 worktree, setup marker, 부모·선택 서브모듈 원격 branch, Jira fields/transition, final snapshot을 검증한다.
- [ ] 같은 명령 재실행, setup 실패, 부모 push 후 서브모듈 push 실패, Jira write 실패, snapshot 실패, 만료 token, 상태 drift, 동시 apply를 검증한다.
- [ ] description·empty E2E는 HOME과 모든 XDG 경로에 Jira config가 없는 상태에서 성공하고 Jira HTTP 요청이 0회인지 검증한다.
- [ ] `cd apps/oma && mise exec -- go test ./tests -run TestPrepEndToEnd -count=1 -v`로 Red를 확인한 뒤 최소 wiring으로 Green을 만든다.
- [ ] `cd apps/oma && mise exec -- go test -race ./...`를 실행한다.
- [ ] `test(oma): verify prep workflow end to end`로 커밋한다.

### Task 9: 자동완성과 사용자 문서를 추가한다

**Files:**

- Create: `apps/oma/completions/_oma`
- Create: `apps/oma/completions/oma.bash`
- Create: `apps/oma/README.md`
- Modify: `apps/oma/.mise.toml`
- Modify: `README.md`
- Modify: `docs/README_ko.md`

**Interfaces:** zsh와 bash는 `prep`, 모든 정적 flag, Git base, worktree path, submodule, branch type, Product type config key를 가능한 범위에서 동적으로 제안한다. completion은 Jira 네트워크 호출을 수행하지 않는다.

- [ ] `completion-check`를 먼저 `.mise.toml`에 추가하고 빈 completion 파일로 `zsh -n`, `bash -n`, 필수 token 검색이 실패하는 Red를 확인한다.
- [ ] 두 completion과 `oma prep --help` 예시, 세 입력 모드, dry-run의 로컬 쓰기, plan/apply, XDG, 마이그레이션, 실패·재실행을 README에 문서화한다.
- [ ] 루트 두 README의 실제 표 구조에 `oma`를 추가한다.
- [ ] 실제 임시 repo에서 base·worktree·submodule 후보 생성 함수를 호출해 중복과 정렬을 눈으로 확인한다.
- [ ] `cd apps/oma && mise run completion-check && mise run fmt-check && mise run lint && mise run test && mise run build`를 실행한다.
- [ ] `docs(oma): document prep command`로 커밋한다.

### Task 10: CI와 릴리스 등록을 연결한다

**Files:**

- Create: `.github/workflows/oma-ci.yml`
- Modify: `release-please-config.json`
- Modify: `.release-please-manifest.json`

**Interfaces:** `oma-ci.yml`은 `apps/oma/**`, 루트 `.mise.toml`, 자기 workflow 변경에만 반응하고 `fmt-check → lint → test → build → completion-check`를 실행한다. release-please package는 `apps/oma`, 초기 manifest는 `0.0.0`이다.

- [ ] 기존 workflow에 흡수할지 비교하고, 앱별 working directory·path filter가 다른 기존 저장소 구조 때문에 독립 `oma-ci.yml`을 선택했다는 근거를 PR 본문에 남긴다.
- [ ] workflow와 release 파일을 수정하기 전 `rg -n 'apps/oma|oma-ci' .github release-please-config.json .release-please-manifest.json`이 일치하지 않는 정적 Red를 기록한다.
- [ ] `actions/checkout@v6`, `jdx/mise-action@v4`, `working-directory: apps/oma`를 기존 앱 workflow와 같은 구조로 작성한다.
- [ ] `ruby` 또는 저장소에서 사용하는 YAML parser로 workflow를 파싱하고 `python3 -m json.tool`로 두 JSON을 검증한다.
- [ ] `cd apps/oma && mise run fmt-check && mise run lint && mise run test && mise exec -- go test -race ./... && mise run build && mise run completion-check`를 실행한다.
- [ ] 루트에서 `mise run version-check`, `mise run runtime-channel-check`, `mise run lint`, `git diff --check`를 실행한다. 공통 release workflow와 conformance 스크립트는 glob으로 앱을 자동 발견하므로 수정하지 않는다.
- [ ] 변경 범위의 PII·시크릿을 알려진 테스트 매치와 함께 교차 검증하고 모든 커밋 제목이 저장소 Conventional Commit 규칙을 만족하는지 검사한다.
- [ ] `ci(oma): register command checks and release`로 커밋한다.

## 라이브 읽기 전용 검증

코드·Formula·설치 배선이 모두 완료된 뒤에만 사용자가 지정한 Jira 티켓과 저장소로 다음 검증을 수행한다. 실제 키와 경로는 문서나 커밋에 남기지 않고 환경변수로 전달한다.

```sh
oma prep "$OMA_SMOKE_ISSUE_KEY" \
  --repo "$OMA_SMOKE_REPO" \
  --base "$OMA_SMOKE_BASE" \
  --dry-run \
  --json
```

- JSON schema와 `required_inputs`를 검사한다.
- snapshot과 plan 파일은 `0600`, 부모 디렉터리는 `0700`인지 검사한다.
- Git branch·worktree·remote ref와 Jira fields가 바뀌지 않았음을 각각 직접 조회한다.
- `git fetch`, snapshot 교체, plan 생성은 dry-run의 의도된 로컬 상태 변경으로 증거에 구분해 적는다.
- 실제 Jira write는 사용자가 준비 작업 적용을 명시적으로 승인한 별도 실행에서만 검증한다.
