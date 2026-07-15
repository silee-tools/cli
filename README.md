# cli

[한국어 (Korean)](docs/README_ko.md)

Monorepo of personal CLI tools under the `silee-tools` GitHub organization. Each tool lives in its own self-contained directory under `apps/` with its own `.mise.toml`, README, and tests. Tools are released independently using a `<tool>/v<MAJOR>.<MINOR>.<PATCH>` tag prefix scheme.

## Tools

| Tool | Language | Description |
|------|----------|-------------|
| [git-tidy](apps/git-tidy/) | Go | Cleans up local git branches that are done or stale — found by gone-upstream, merged, or staleness signals. |
| [git-update-default](apps/git-update-default/) | Go | Switch the current repo to the latest remote default branch. |
| [jg / jgw](apps/jg/) | Go | Frecency-based CLI for quickly jumping to git repositories (jg) and to worktrees within a repo (jgw). |
| [oma](apps/oma/) | Go | Prepares approved agent work plans, Git worktrees and branches, and Jira work-start state. |
| [totp](apps/totp/) | Go | macOS Keychain-backed TOTP code generator (macOS only). |

## Tech Stack

- Go monorepo (each tool independent)
- Task Runner: [mise](https://mise.jdx.dev/) (per-tool `.mise.toml`)
- CI: GitHub Actions with `paths:` filters per tool
- Distribution: [silee-tools/homebrew-tap](https://github.com/silee-tools/homebrew-tap) (separate repo)

## Repository Layout

```
silee-tools/cli/
├── apps/                       # one subdirectory per tool, self-contained
│   └── <tool>/
│       ├── .mise.toml
│       ├── .goreleaser.yaml
│       ├── README.md
│       └── ...
├── docs/                       # shared Korean docs and migration notes
├── scripts/                    # commit lint and release helper scripts
├── .github/workflows/          # per-tool CI, commit lint, release automation
├── .commitlintrc.yml           # Conventional Commits policy for CI
├── .editorconfig               # shared editor defaults for generated text files
├── .gitattributes              # LF normalization for docs, configs, and scripts
├── .release-please-manifest.json
├── release-please-config.json  # per-tool release-please packages
├── .mise.toml                  # common dev tools only
└── README.md
```

## Getting Started

Clone the repository and pick a tool to work on:

```bash
git clone git@github.com:silee-tools/cli.git
cd cli/apps/<tool>
mise trust ../.. .
mise run test
```

`mise` requires explicit trust for both the repository root config and the
per-tool config before running tasks in a fresh clone.

## Commit Policy

All PR and `main` push commits are checked by `Commit Lint`. Use a Conventional
Commit header such as `feat(jg): add cleanup scheduler` or
`ci(git-tidy): install zsh in CI`; non-conforming commits block the PR until
they are amended.

## Releases

Release tags use a per-tool prefix:

```
<tool>/v<MAJOR>.<MINOR>.<PATCH>
```

For example, `jg/v0.1.0` identifies a release of the `jg` tool. The shared release workflow builds the tools listed in release-please's `paths_released` output.
