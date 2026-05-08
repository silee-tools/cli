# unid (Unicode Diagram)

CLI tool for rendering ASCII diagrams using Unicode box-drawing characters.

A text-based alternative to editors like Monodraw or ASCIIFlow.
Renders precise Unicode box-drawing diagrams from a simple DSL via stdin.

[한국어 (Korean)](docs/README.ko.md)

## Install

### Homebrew (macOS)

```sh
brew install silee-tools/tap/unid
```

### Build from source

```sh
git clone https://github.com/silee-tools/unid.git
cd unid
go build -o unid ./cmd/unid
# optionally install to $GOPATH/bin:
go install ./cmd/unid
```

## Usage

```sh
echo "..." | unid          # Render diagram from stdin
echo "..." | unid list     # List objects in diagram
echo "..." | unid lint     # Lint DSL input
unid guide                 # Show usage guide
```

## Example

```sh
echo 'canvas auto
collision off
box 0 0 10 3 id=a c=Client
box 16 0 10 3 id=b c=Server
box 32 0 8 3 id=c c=DB
arrow a.r b.l
arrow b.r c.l' | unid
```

```
┌──────────┐    ┌──────────┐    ┌────────┐
│          │    │          │    │        │
│Client    │───▶│Server    │───▶│DB      │
│          │    │          │    │        │
└──────────┘    └──────────┘    └────────┘
```

## Features

- **Auto width calculation** — display-column aware rendering for CJK characters (한글, 漢字, かな)
- **Multiple border styles** — single, double, bold, round, dashed, and more
- **DSL-based** — declaratively define diagrams as text
- **Lint support** — detect errors and warnings in DSL input

See `unid guide` for full documentation.

## Use with AI

```
Draw a system architecture diagram using the unid CLI. Refer to `unid guide` for the DSL syntax.
```

## License

[MIT](LICENSE)
