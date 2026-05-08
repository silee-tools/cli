package main

import "fmt"

func printGuide() {
	fmt.Print(`unid - Unicode Diagram Renderer

A text-based alternative to ASCII diagram editors (Monodraw, ASCIIFlow, etc).
Renders precise Unicode box-drawing diagrams from a simple DSL via stdin.

USAGE:
  echo "..." | unid          Render from stdin (default)
  echo "..." | unid list    List objects in diagram
  echo "..." | unid lint    Lint DSL for errors/warnings
  unid guide                Show this guide

DSL SYNTAX:
  Lines starting with # are comments. Blank lines are ignored.
  Commands are case-insensitive. Each command is on its own line.

  GLOBAL SETTINGS (must appear before objects):
    collision on|off                       Required
    overflow <mode>                        Global overflow mode (optional)
    align <align>                          Global alignment (optional)
    arrowhead <char>                       Global arrowhead family (optional)

  OBJECTS:
    box <col> <row> <w> <h> [id=<name>] [style(s)=<style>] [overflow(o)=<mode>]
        [align(a)=<align>] [legend-pos(lp)=top(t)|bottom(b)]
        [legend-overflow(lo)=<mode>] [legend-align(la)=<align>]
        [legend(lg)=<text>] [content(c)=<text>]
      - W, H: inner size (border excluded). Total: (W+2) x (H+2)
      - "rect" is accepted as an alias for "box"
    text <col> <row> [id=<name>] [align(a)=<align>] content(c)=<text>
    hline <col> <row> <length> [id=<name>] [style(s)=<style>] [pos=<pos>]
        [legend-overflow(lo)=<mode>] [legend-align(la)=<align>] [legend(lg)=<text>]
    vline <col> <row> <length> [id=<name>] [style(s)=<style>] [pos=<pos>]
        [legend-overflow(lo)=<mode>] [legend-align(la)=<align>] [legend(lg)=<text>]
    arrow <src_id>.<side> <dst_id>.<side> [head=<char>] [both] [pos=<pos>]
        [legend-overflow(lo)=<mode>] [legend-align(la)=<align>] [legend(lg)=<text>]

    id= names: alphanumeric, underscore, hyphen only
    CJK characters (한글, 漢字, かな) take 2 display columns
    Canvas size is always auto-computed from object bounds.

BOX STYLES (style(s)= for box):
  light(l, default):  ┌─┐ │ └─┘
  heavy(h):           ┏━┓ ┃ ┗━┛
  double(d):          ╔═╗ ║ ╚═╝
  rounded(r):         ╭─╮ │ ╰─╯

LINE STYLES (style(s)= for hline/vline):
  light(l, default):  ─ │
  heavy(h):           ━ ┃
  double(do):         ═ ║
  dash(da):           ╌ ╎

CONTENT & LEGEND:
  content(c)=         Text inside the object (box inner area, text object)
  legend(lg)=         Text outside/near the object (box, hline, vline, arrow)
  content(c)= and legend(lg)= must be the last options on a line.
  Use <br> for multiline text. Leading/trailing whitespace per line is trimmed.

OVERFLOW MODES (overflow(o)= / legend-overflow(lo)= / global "overflow"):
  ellipsis(el, default): Truncate with "prefix..{N}" where N=truncated display width
  overflow(o):           Content overwrites borders
  hidden(h):             Truncate without indicator
  error(er):             Return error if content exceeds width

ALIGNMENT (align(a)= / legend-align(la)= / global "align"):
  left(l, default):   Left-aligned (right side truncated/overflows)
  center(c):          Center-aligned (both sides truncated/overflow)
  right(r):           Right-aligned (left side truncated/overflows)
  Note: arrow legend-align defaults to center(c); all others default to left(l).

LEGEND POSITION:
  Box:      legend-pos(lp)=top(t)|bottom(b)         (default: top)
  HLine:    pos=top(t)|bottom(b)|left(l)|right(r)   (default: top)
  VLine:    pos=left(l)|right(r)|top(t)|bottom(b)   (default: right)
  Arrow:    pos=top(t)|bottom(b)|left(l)|right(r)|auto(a)  (default: auto)

ARROWS:
  Arrows connect objects by id and side (anchor point).
  Any object with id= can be an arrow endpoint (box, text, hline, vline).
  Routing is automatic based on source/destination sides.

  Syntax: arrow <src_id>.<side> <dst_id>.<side> [head=<char>] [both] [lg=<text>]

  Options:
    head=<char>       Arrowhead family (auto-resolves direction). Valid families:
                      ▶◀▲▼ (default)  →←↑↓  ▷◁△▽  ⇒⇐⇑⇓
    both              Bidirectional arrow (arrowhead on both ends)
    lg=<text>         Legend text near the arrow

  Arrowhead priority: per-arrow head= > global arrowhead command > default (▶◀▲▼)
  Direction auto-resolved: head=▶ on a vertical arrow renders as ▼ or ▲

  Route types (auto-selected):
    Straight:     ──▶        (opposite sides, aligned)
    L-shaped:     ──┐        (perpendicular sides, favorable)
                    ▼
    Z-shaped:     ──┐        (same direction, not aligned)
                    └──▶
    U-shaped:     ──┐        (opposite sides, same axis — ㄷ shape)
                    │
                  ◀─┘
    Self-loop:    ──┐        (same object, different sides — ㄷ shape)
                    │
                ▲───┘

  Source anchor: 1 cell outside border (arrow starts here)
  Dest anchor:   1 cell outside border (arrowhead does not overwrite border)

RENDERING:
  2-pass rendering: structure first (borders, lines), then text (c=, lg=, text).
  Text content always renders on top of structural elements.

TIPS:
  - Arrow legend is placed near the midpoint of the longest segment.
    Multiple arrows in the same area may have overlapping legends.
  - For precise label positioning, use text objects instead of arrow legends:
      arrow src.b dst.t
      text 15 7 c=my label
  - Legend text exceeding canvas bounds is truncated per legend-overflow mode
    (default: ellipsis). Use lo=overflow to allow overflow beyond canvas edge.

EXAMPLE:
  input:
    echo "collision off
    align c
    # Boxes with legend and arrow labels
    box 0 1 16 1 id=api s=d lg=Server c=API Gateway
    box 26 1 12 1 id=web c=Web Client
    box 0 9 16 1 id=auth s=r c=Auth 인증
    box 26 9 12 1 id=db s=h align=r c=Data Store
    # Arrows with legend labels and custom head
    arrow api.r web.l both head=▶ lg=HTTP
    arrow api.b auth.t lg=verify
    arrow web.b db.t lg=query
    arrow auth.r db.l lg=sync
    # Self-loop on db
    arrow db.r db.b pos=r lg=backup
    # Separator + overflow demos
    hline 0 17 50 s=dash lg=Features
    text 0 19 c=ellipsis:
    box 10 19 10 1 c=LongServiceName
    text 24 19 c=overflow:
    box 34 19 10 1 overflow=overflow c=LongServiceName
    text 0 22 c=hidden:
    box 10 22 10 1 overflow=hidden c=LongServiceName" | unid

  output:
    Server
    ╔════════════════╗ HTTP   ┌────────────┐
    ║  API Gateway   ║◀──────▶│ Web Client │
    ╚════════════════╝        └────────────┘
             │                       │
             │                       │
             │verify                 │query
             │                       │
             ▼                       ▼
    ╭────────────────╮ sync   ┏━━━━━━━━━━━━┓
    │   Auth 인증    │───────▶┃  Data Store┃──┐
    ╰────────────────╯        ┗━━━━━━━━━━━━┛  │
                                     ▲        │
                                     │        │
                                     └────backup

    Features
    ╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌

    ellipsis: ┌──────────┐  overflow: ┌──────────┐
              │LongSer..8│            │LongServiceName
              └──────────┘            └──────────┘
    hidden:   ┌──────────┐
              │LongServic│
              └──────────┘

`)
}
