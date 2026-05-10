# 릴리스 자동화 가이드

## 흐름 (release-please + GoReleaser 정석 패턴)

본 저장소의 릴리스는 main 으로 들어오는 Conventional Commits 가 자연스럽게 트리거하는 흐름을 따른다. 사람이 손으로 태그를 만들지 않고, 대신 release-please-action 이 항상 최신 상태로 유지해 두는 도구별 Release PR 을 review 하고 merge 하는 것이 릴리스 결정이다.

전체 단계는 다음과 같다.

1. **개발자가 main 에 `feat`/`fix` 등 Conventional Commits 를 누적한다.** `commitlint.yml` workflow 가 매 PR 과 main push 마다 형식을 강제한다.
2. **release-please-action 이 main push 마다 실행되어, 도구별로 다음 버전 bump + CHANGELOG 변경분이 들어 있는 Release PR 을 생성하거나 기존 PR 을 갱신한다.** `release-please-config.json` 의 packages 항목과 `.release-please-manifest.json` 의 현재 버전을 결합해 다음 버전을 계산한다.
3. **개발자가 Release PR 의 본문을 검토한다.** 어느 도구가 어떤 commit 들을 받아 어떤 버전으로 가는지가 PR 본문에 그대로 보인다. 의도와 다르면 PR 을 수정하거나(commit 추가/회수) 그냥 두고 Release PR merge 만 미룬다.
4. **개발자가 Release PR 을 merge 한다.** release-please-action 이 즉시 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 태그를 만들고, 빈 GitHub Release(노트는 CHANGELOG 변경분으로 자동 채워짐) 를 생성한다. CHANGELOG.md 갱신은 PR merge 와 함께 main 에 들어가 있다.
5. **같은 release-please.yml 안의 build-and-upload matrix job 이 도구별로 한 번씩 실행된다.** 각 도구의 `.goreleaser.yaml` 로 GoReleaser 가 dist 산출물(tar.gz × 4 + checksums.txt) 을 만든 뒤 release-please 가 만들어 둔 release 에 `gh release upload` 로 첨부한다. GoReleaser 는 `release.disable: true` 로 release 객체 자체를 건드리지 않는다.
6. **같은 job 마지막 step 이 homebrew-tap 의 `Formula/<tool>.rb` 의 sha256 placeholder 와 version 라인을 새 값으로 자동 갱신하고 commit + push 한다.** `HOMEBREW_TAP_TOKEN` secret 이 설정된 경우에만 동작하며, 여러 도구 릴리스가 가까운 시점에 tap 저장소를 갱신하더라도 non-fast-forward 경합은 `git pull --rebase` 후 재시도한다. 미설정 시 step 자체가 skip 되고 `notice` 로 수동 갱신 안내가 출력된다.
7. **사용자 머신에서 `brew update && brew upgrade silee-tools/tap/<tool>` 로 새 버전 설치.**

## release-please 의 동작 원리

release-please 는 main 의 commit history 를 보고 각 도구별로 마지막 release 태그 이후의 Conventional Commits 를 분석한다. `feat:` 가 하나라도 있으면 minor bump, `fix:` 만 있으면 patch bump, `feat!:` 또는 본문에 `BREAKING CHANGE:` 가 있으면 major bump 하는 식이다.

도구별 격리는 `release-please-config.json` 의 `packages` 객체로 표현된다. 각 키가 monorepo 안의 도구 디렉토리 경로(`apps/jg`) 이며, release-please 는 해당 경로를 건드린 commit 만 그 도구의 이력에 포함시킨다. `tag-separator: "/"` 와 `include-component-in-tag: true` 설정 때문에 태그 형식이 `<package-name>/v<X.Y.Z>` 가 되어 우리 prefix 스킴과 정확히 일치한다.

`release-type: simple` 은 도구의 source 안에 version 파일을 두지 않고 manifest 만으로 버전을 추적하는 모드다. 우리는 빌드 시 ldflags(`-X main.version=...`) 로 버전을 주입하므로 source 안 version 파일이 불필요하다.

`bump-minor-pre-major: true` 는 `0.x` 구간에서 `feat!:` 도 minor bump(0.1.0 → 0.2.0) 로 처리하라는 옵션이다. 1.0 미만 도구의 breaking 변경을 minor 로 흡수해 `1.0` 까지의 도달 시점을 사람이 직접 결정하게 한다.

manifest 의 초기값은 마지막 릴리스된 버전을 그대로 둔다. 다음 commit 의 종류(feat/fix 등) 에 따라 release-please 가 patch/minor/major 로 자동 bump 한다.

## 자동/수동 단계 표

| 단계 | 자동 / 수동 | 행위자 |
|---|---|---|
| Conventional Commits 강제 검사 | 자동 | commitlint.yml |
| Release PR 생성/갱신 | 자동 | release-please-action |
| Release PR review + merge | **수동** | 본인 |
| 태그 + 빈 GitHub Release 생성 | 자동 | release-please-action (PR merge 후) |
| CHANGELOG.md main 반영 | 자동 | release-please (Release PR 안) |
| 도구별 artifact 빌드 + 첨부 | 자동 | release-please.yml 의 build-and-upload job |
| homebrew-tap formula 갱신 + push | 자동 | 같은 job 마지막 step |
| `brew install/upgrade` | 수동 | 본인 |

사람의 결정 지점은 단 하나, "Release PR 을 merge 할지" 이며 그 외에는 모두 자동이다.

## HOMEBREW_TAP_TOKEN 설정 (1회성)

silee-tools/homebrew-tap 레포에 push 권한을 가진 fine-grained Personal Access Token 을 만들어 silee-tools/cli 의 repository secret 으로 등록한다.

### Token 생성

GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token

- Token name: `silee-tools-cli-homebrew-tap-bot`
- Expiration: 1년 (만료 전 갱신 알림 캘린더 등록 권장)
- Resource owner: `silee-tools`
- Repository access: Only select repositories → `silee-tools/homebrew-tap`
- Repository permissions:
  - **Contents: Read and write** (필수)
  - Metadata: Read-only (자동 부여)

### Secret 등록

```bash
gh secret set HOMEBREW_TAP_TOKEN \
  --repo silee-tools/cli \
  --body "<생성한_PAT>"
```

또는 GitHub UI: silee-tools/cli → Settings → Secrets and variables → Actions → New repository secret → Name: `HOMEBREW_TAP_TOKEN`, Value: 토큰

## Self-hosted 러너 사용 (선택)

build-and-upload job 의 runner 는 다음 우선순위로 결정된다.

- `apps/totp` 매트릭스: `vars.RUNNER_MACOS` 가 설정되어 있으면 그 값을, 아니면 `macos-latest`. (totp 만 cgo + Keychain 으로 macOS 가 강제됨)
- 그 외 매트릭스: `vars.RUNNER_LINUX` 가 설정되어 있으면 그 값을, 아니면 `ubuntu-latest`.

자체 호스팅 macOS / Linux 러너가 있으면 GitHub Actions 의 self-hosted 러너 라벨을 `vars.RUNNER_MACOS` / `vars.RUNNER_LINUX` 에 등록한다. 미설정 시 GitHub-hosted 로 동작한다.

```bash
gh variable set RUNNER_LINUX --repo silee-tools/cli --body "self-hosted"
gh variable set RUNNER_MACOS --repo silee-tools/cli --body "self-hosted"
```

## 도구명 → formula 파일명 매핑

도구 디렉토리명과 formula 파일명이 같다.

| 도구 (apps/<tool>) | Formula |
|---|---|
| jg | `jg.rb` |
| totp | `totp.rb` |
| saml2aws-auto | `saml2aws-auto.rb` |

신규 도구 첫 릴리스 시 homebrew-tap 에 formula 가 없으면 자동 갱신 step 은 skip 되며 `notice` 가 출력된다 — 이때만 수동으로 formula 를 작성해 push 한 뒤 다음 릴리스부터 자동 갱신이 동작한다.

## 신규 도구 추가 절차

1. `apps/<new-tool>/` 디렉토리에 도구 코드 + `.mise.toml` + README + (Go 라면 `.goreleaser.yaml`) 작성
2. `.github/workflows/<new-tool>-ci.yml` paths 필터 CI 추가
3. `release-please-config.json` 의 `packages` 에 `"apps/<new-tool>": {"package-name": "<new-tool>"}` 추가
4. `.release-please-manifest.json` 에 `"apps/<new-tool>": "0.0.0"` 추가
5. `homebrew-tap/Formula/<new-tool>.rb` 골격 작성 (sha256 placeholder, prebuilt URL 패턴)
6. main 으로 `feat(<new-tool>): initial release` commit push → release-please 가 0.1.0 Release PR 생성 → merge → 첫 릴리스

## 검증 절차 (첫 적용 시)

1. `HOMEBREW_TAP_TOKEN` secret 설정
2. 작은 도구 하나에 `docs(<tool>): trivial doc tweak` 같은 commit 을 main 에 push
3. Actions 탭에서 release-please-action 실행 확인 → 자동 생성된 Release PR 의 본문 점검 (CHANGELOG 변경분 + manifest version bump)
4. Release PR merge → 같은 workflow 가 다시 돌아 build-and-upload job 이 실행되는지 확인
5. 새 GitHub Release 페이지에서 tar.gz + checksums.txt 첨부 확인
6. silee-tools/homebrew-tap 의 새 commit (`chore(<tool>): bump to v<X.Y.Z>...`) 확인
7. 본인 머신에서 `brew update && brew upgrade silee-tools/tap/<tool>` 정상 동작

7번까지 통과하면 마이그레이션 완료.

## 의도적으로 자동화하지 않은 항목

| 항목 | 사유 |
|---|---|
| 버전 결정의 최종 승인 | release-please 가 다음 버전을 추천하지만, 본인이 Release PR merge 시점에 최종 승인. semantic-release 류의 fully-automatic versioning 은 commit 메시지 형식 의존이 깨지기 쉬워 도입하지 않음 |
| 신규 도구 formula 골격 작성 | 첫 릴리스 시 formula 골격은 본인이 직접 작성. 두 번째 릴리스부터 sha256/version 자동 갱신 |
| `brew install/upgrade` | 본인 머신 동작이므로 자동화 대상 아님 |
