# cli

[한국어 (Korean)](docs/README_ko.md)

Monorepo of personal CLI tools under the `silee-tools` GitHub organization. Each tool lives in its own self-contained directory under `apps/` with its own `.mise.toml`, README, and tests. Tools are released independently using a `<tool>/v<MAJOR>.<MINOR>.<PATCH>` tag prefix scheme.

## Tools

| Tool | Language | Description |
|------|----------|-------------|
| [jg](apps/jg/) | Go | Frecency-based CLI for quickly jumping to git repositories. |
| [saml2aws-auto](apps/saml2aws-auto/) | Go | Auto-injects a TOTP MFA code into `saml2aws` AzureAD login and checks session expiry from zsh. |
| [totp](apps/totp/) | Go | macOS Keychain-backed TOTP code generator (macOS only). |

## Tech Stack

- Go monorepo (each tool independent)
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
├── .github/workflows/ # per-tool CI + shared release-please.yml
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
