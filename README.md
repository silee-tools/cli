# cli

[한국어 (Korean)](docs/README_ko.md)

Monorepo of personal CLI tools under the `silee-tools` GitHub organization. Each tool lives in its own self-contained directory under `apps/` with its own `.mise.toml`, README, and tests. Tools are released independently using a `<tool>/v<MAJOR>.<MINOR>.<PATCH>` tag prefix scheme.

## Tools

| Tool | Language | Description |
|------|----------|-------------|
| [saml2aws-auto](apps/saml2aws-auto/) | Go | Auto-injects a TOTP MFA code into `saml2aws` AzureAD login. |
| [totp](apps/totp/) | Go | macOS Keychain-backed TOTP code generator (macOS only). |
| (other apps will be migrated in subsequent commits) | | |

## Tech Stack

- Multi-language monorepo (Bash, Bun/TypeScript, Go, Zsh)
- Task Runner: [mise](https://mise.jdx.dev/) (per-tool `.mise.toml`)
- CI: GitHub Actions with `paths:` filters per tool
- Distribution: [silee-tools/homebrew-tap](https://github.com/silee-tools/homebrew-tap) (separate repo)

## Repository Layout

```
silee-tools/cli/
├── apps/              # one subdirectory per tool, self-contained
│   └── <tool>/
│       ├── .mise.toml
│       ├── README.md
│       └── ...
├── .github/workflows/ # per-tool CI + shared release.yml
├── .mise.toml         # common dev tools only
└── README.md
```

## Getting Started

Clone the repository and pick a tool to work on:

```bash
git clone git@github.com:silee-tools/cli.git
cd cli/apps/<tool>
mise run test
```

## Releases

Release tags use a per-tool prefix:

```
<tool>/v<MAJOR>.<MINOR>.<PATCH>
```

For example, `jg/v0.1.0` releases the `jg` tool. The shared release workflow extracts the prefix and builds the matching tool.
