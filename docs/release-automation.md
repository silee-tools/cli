# 릴리스 자동화 가이드

## 흐름

`<tool>/v<MAJOR>.<MINOR>.<PATCH>` 태그 push → `.github/workflows/release.yml` 가 자동으로 다음을 수행한다.

1. 태그 prefix 로 도구 식별 (`apps/<tool>/` 디렉토리 매핑)
2. 빌드 카테고리 분기 — goreleaser / bun / bash
3. multi-arch artifact + `checksums.txt` 생성
4. `gh release create` 로 GitHub Release 에 첨부
5. **homebrew-tap 의 `Formula/<tool>.rb` 의 `sha256` placeholder 와 `version` 라인 자동 갱신 + commit + push**

5단계는 `HOMEBREW_TAP_TOKEN` repository secret 이 설정된 경우에만 수행된다. 미설정 시 GitHub Actions 의 `notice` 로 수동 갱신 안내가 출력된다.

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

## 도구명 → formula 파일명 매핑

대부분 도구 디렉토리명과 formula 파일명이 같지만, `beautiful-mermaid-cli` 만 명령 이름인 `bmm` 을 따른다.

| 도구 (apps/<tool>) | Formula |
|---|---|
| appback | `appback.rb` |
| beautiful-mermaid-cli | `bmm.rb` |
| jg | `jg.rb` |
| mydesk | `mydesk.rb` |
| unid | (formula 없음 — 신규 추가 필요) |
| totp | `totp.rb` |
| saml2aws-auto | `saml2aws-auto.rb` |

신규 도구 첫 릴리스 시 homebrew-tap 에 formula 가 없으면 자동 갱신 step 은 skip 되며 `notice` 가 출력된다 — 이때만 수동으로 formula 를 작성해 push 한 뒤 다음 릴리스부터 자동 갱신이 동작한다.

## 자동화 범위와 한계

자동화되어 있는 것.

- artifact 빌드 / 첨부
- formula sha256 + version 갱신 + commit + push
- 릴리스 노트 (`--generate-notes` 로 commit 메시지 기반)

자동화하지 않은 것 (의도된 결정).

- 버전 결정 — 사용자가 직접 `<tool>/vX.Y.Z` 태그를 만든다. semantic-release 류 도구는 conventional-commits 파싱 의존 + 멱등성 손상 가능성으로 도입하지 않는다.
- changelog 별도 파일 — `--generate-notes` 로 충분하다고 판단. 별도 CHANGELOG.md 가 필요해지는 시점에 재검토.
- formula 내용 자체 작성 — 신규 도구 첫 릴리스 시 formula 골격은 사용자가 작성. 두 번째 릴리스부터 자동 갱신.

## 검증 절차 (첫 적용 시)

1. `HOMEBREW_TAP_TOKEN` secret 설정 후 작은 도구 하나에 patch tag 푸시
2. release.yml 실행 로그에서 `Update homebrew-tap formula` step 통과 확인
3. silee-tools/homebrew-tap 의 새 commit 확인 — `chore(<tool>): bump to v<X.Y.Z> from silee-tools/cli release` 메시지
4. 본인 머신에서 `brew update && brew upgrade silee-tools/tap/<tool>` 후 정상 설치 검증
