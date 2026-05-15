# cli

[English (영어)](../README.md)

`silee-tools` GitHub 조직 아래 개인용 CLI 도구들을 모은 모노레포다. 각 도구는 `apps/` 하위에 자기 완결적인 디렉토리로 들어가 있으며, 자체 `.mise.toml`, README, 테스트를 갖는다. 도구별 릴리스는 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 형식의 태그 prefix 스킴으로 독립적으로 진행한다.

## 도구

| 도구 | 언어 | 설명 |
|------|------|------|
| [git-tidy](../apps/git-tidy/) | zsh | trunk 기반 squash merge 워크플로우에서 upstream 이 사라진 로컬 브랜치를 안전하게 정리. |
| [jg](../apps/jg/) | Go | 디렉토리 frecency 기반 빠른 git 저장소 점프 CLI. |
| [saml2aws-auto](../apps/saml2aws-auto/) | Go | `saml2aws` AzureAD 로그인에 TOTP MFA 코드를 자동 주입하고 zsh 시작 시 세션 만료를 확인. |
| [totp](../apps/totp/) | Go | macOS Keychain 기반 TOTP 코드 생성기 (macOS 전용). |

## 기술 스택

- Go 모노레포 (각 도구 독립)
- Task Runner: [mise](https://mise.jdx.dev/) (도구별 `.mise.toml`)
- CI: GitHub Actions, 도구별 `paths:` 필터
- 배포: [silee-tools/homebrew-tap](https://github.com/silee-tools/homebrew-tap) (별도 레포)

## 저장소 구조

```
silee-tools/cli/
├── apps/                       # 도구당 하위 디렉토리 1개, 자기 완결적
│   └── <tool>/
│       ├── .mise.toml
│       ├── .goreleaser.yaml
│       ├── README.md
│       └── ...
├── docs/                       # 공통 한국어 문서와 마이그레이션 노트
├── scripts/                    # commit lint 와 릴리스 보조 스크립트
├── .github/workflows/          # 도구별 CI, commit lint, 릴리스 자동화
├── .commitlintrc.yml           # CI Conventional Commits 정책
├── .editorconfig               # 공통 에디터 저장 정책
├── .gitattributes              # 문서/설정/스크립트 LF 정규화
├── .release-please-manifest.json
├── release-please-config.json  # 도구별 release-please 패키지 설정
├── .mise.toml                  # 공통 개발 도구만
└── README.md
```

## 시작하기

레포를 클론하고 작업할 도구를 고른다:

```bash
git clone git@github.com:silee-tools/cli.git
cd cli/apps/<tool>
mise run test
```

## 커밋 정책

모든 PR 과 `main` push commit 은 `Commit Lint` 로 검증한다. `feat(jg): add cleanup scheduler` 또는 `ci(saml2aws-auto): install zsh in CI` 같은 Conventional Commit header 를 사용해야 하며, 형식에 맞지 않는 commit 은 amend 하기 전까지 PR 을 차단한다.

## 릴리스

릴리스 태그는 도구별 prefix 를 붙인다:

```
<tool>/v<MAJOR>.<MINOR>.<PATCH>
```

예를 들어 `jg/v0.1.0` 은 `jg` 도구의 릴리스다. 공통 릴리스 워크플로우가 prefix 를 추출해 해당 도구를 빌드한다.
