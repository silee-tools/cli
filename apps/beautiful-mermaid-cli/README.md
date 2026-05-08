# beautiful-mermaid-cli

[한국어 (Korean)](docs/README_ko.md)

A thin CLI wrapper around beautiful-mermaid for rendering Mermaid diagrams as ASCII art.

## Tech Stack

- Runtime: [Bun](https://bun.sh/)
- Language: TypeScript
- Core: [beautiful-mermaid](https://github.com/nicholasgasior/beautiful-mermaid)

## Install

```bash
brew install silee-tools/tap/bmm
```

## Getting Started

```bash
# Render from stdin
printf 'graph LR\n  A --> B\n  B --> C\n' | bmm

# Render a file
bmm diagram.mmd

# SVG output
bmm --svg diagram.mmd

# Pure ASCII (no Unicode box-drawing)
bmm --ascii diagram.mmd

# Show help with Mermaid syntax tips
bmm --help
```

## Development

```bash
bun install
printf 'graph LR\n  A --> B\n' | bun src/cli.ts
bun link  # global install from source
```
