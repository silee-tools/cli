# cli

[English (영어)](../README.md)

`silee-tools` GitHub 조직 아래 개인용 CLI 도구들을 모은 모노레포다. 각 도구는 `apps/` 하위에 자기 완결적인 디렉토리로 들어가 있으며, 자체 `.mise.toml`, README, 테스트를 갖는다. 도구별 릴리스는 `<tool>/v<MAJOR>.<MINOR>.<PATCH>` 형식의 태그 prefix 스킴으로 독립적으로 진행한다.

## 도구

| 도구 | 언어 | 설명 |
|------|------|------|
| [saml2aws-auto](../apps/saml2aws-auto/) | Go | `saml2aws` AzureAD 로그인에 TOTP MFA 코드를 자동 주입. |
| [totp](../apps/totp/) | Go | macOS Keychain 기반 TOTP 코드 생성기 (macOS 전용). |
| (그 외 도구는 이후 커밋으로 이주 예정) | | |

## 기술 스택

- 다중 언어 모노레포 (Bash, Bun/TypeScript, Go, Zsh)
- Task Runner: [mise](https://mise.jdx.dev/) (도구별 `.mise.toml`)
- CI: GitHub Actions, 도구별 `paths:` 필터
- 배포: [silee-tools/homebrew-tap](https://github.com/silee-tools/homebrew-tap) (별도 레포)

## 저장소 구조

```
silee-tools/cli/
├── apps/              # 도구당 하위 디렉토리 1개, 자기 완결적
│   └── <tool>/
│       ├── .mise.toml
│       ├── README.md
│       └── ...
├── .github/workflows/ # 도구별 CI + 공통 release.yml
├── .mise.toml         # 공통 개발 도구만
└── README.md
```

## 시작하기

레포를 클론하고 작업할 도구를 고른다:

```bash
git clone git@github.com:silee-tools/cli.git
cd cli/apps/<tool>
mise run test
```

## 릴리스

릴리스 태그는 도구별 prefix 를 붙인다:

```
<tool>/v<MAJOR>.<MINOR>.<PATCH>
```

예를 들어 `jg/v0.1.0` 은 `jg` 도구의 릴리스다. 공통 릴리스 워크플로우가 prefix 를 추출해 해당 도구를 빌드한다.
