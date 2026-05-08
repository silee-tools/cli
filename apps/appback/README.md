# appback

[한국어 (Korean)](docs/README_ko.md)

Mac app settings backup and restore CLI tool.

Backup and restore macOS application settings, preferences, and keychain items with an interactive terminal UI powered by [gum](https://github.com/charmbracelet/gum).

## Tech Stack

- Bash
- [gum](https://github.com/charmbracelet/gum) — interactive terminal UI
- [bats](https://github.com/bats-core/bats-core) — testing framework

## Getting Started

```bash
# Install dependencies
brew install gum

# Clone and use
git clone https://github.com/silee9019/appback.git
cd appback
./appback --help
```

## Usage

```bash
# List supported apps
./appback list

# Export app settings
./appback export dia

# Import app settings
./appback import dia ~/Downloads/dia-backup-2026-04-08.tar.gz

# Interactive mode (no arguments)
./appback export
./appback import
```
