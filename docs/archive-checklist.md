# 구 silee-tools 5개 레포 archive 체크리스트 (Task #8)

작성일: 2026-05-09
상태: 사용자 확인 게이트 (실행 전 검토)

## 0. 진행 시점

이 체크리스트는 다음 모든 조건이 만족된 뒤에만 실행한다.

- silee-tools/cli 모노레포에 첫 릴리스 태그(예: `appback/v0.2.4`, `jg/v0.1.28` 등) 가 push 되어 GitHub Releases 에 노출
- silee-tools/homebrew-tap 의 formula sha256 이 실제 archive 해시로 갱신되고 push 됨 (Task #5 의 follow-up)
- 본인 머신에서 `brew reinstall silee-tools/tap/<tool>` 가 새 source URL 로 정상 동작 확인
- totp / saml2aws-auto 의 경우 Task #7 Go 재작성 + .zshrc 마이그레이션 가이드(docs/migration-zshrc.md) 의 단계 4 회귀 테스트까지 통과

## 1. 대상 레포

| 레포 | 처리 |
|---|---|
| silee-tools/appback | archive |
| silee-tools/beautiful-mermaid-cli | archive |
| silee-tools/jg | archive |
| silee-tools/mydesk | archive |
| silee-tools/unid | archive |
| silee9019/zsh-plugins (totp, saml2aws-auto 디렉토리만) | 디렉토리 제거 + README 갱신 (레포 자체는 유지) |
| silee-tools/homebrew-tap | 그대로 유지 (별도 레포로 계속 운영) |

## 2. 단계별 절차

### 2.1 각 archive 대상 레포에 안내문 추가 (별도 PR/commit)

```
이 레포는 silee-tools/cli 모노레포로 통합되었습니다.
새 위치: https://github.com/silee-tools/cli/tree/main/apps/<tool>
```

각 레포 README.md 최상단에 한 줄 추가하고 commit + push.

### 2.2 GitHub archive 처리

```bash
gh repo archive silee-tools/appback --yes
gh repo archive silee-tools/beautiful-mermaid-cli --yes
gh repo archive silee-tools/jg --yes
gh repo archive silee-tools/mydesk --yes
gh repo archive silee-tools/unid --yes
```

archive 후 레포는 read-only 가 된다. 기존 issue/PR 은 그대로 유지되어 archive 마커가 붙는다.

### 2.3 zsh-plugins 의 totp / saml2aws-auto 디렉토리 제거

```bash
cd ~/ResilioSync/silee-drive/Repositories/silee9019/zsh-plugins
git rm -r totp saml2aws-auto
# README.md 의 plugin 표에서 두 행 제거
git commit -m "chore: drop totp/saml2aws-auto (이주처: silee-tools/cli)"
git push
```

### 2.4 사후 검증

- `brew reinstall silee-tools/tap/{appback,bmm,jg,mydesk}` 모두 정상 동작
- `~/.zshrc` 의 zinit multisrc 에서 totp/saml2aws-auto 빠진 상태로 새 셸 정상 로드
- `saml2aws-auto-login` 1회 실행하여 회사 AWS 콘솔 접근 회귀 없음
- 기존 archive 된 레포 페이지에서 README 의 이주 안내 + GitHub archive 배지가 표시되는지 육안 확인

## 3. 되돌리기 (필요 시)

archive 는 GitHub UI 또는 `gh repo unarchive <repo>` 로 즉시 되돌릴 수 있다. 이주 안내 README commit 도 일반 PR 로 revert 가능. 따라서 이 단계는 비가역이 아니지만, 시점이 빠르면 brew/zshrc 마이그레이션 회귀를 빨리 잡지 못해 일상 업무가 막힐 수 있어 §0 의 모든 조건을 만족한 뒤에만 진행한다.
