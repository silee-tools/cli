#!/usr/bin/env bun

import { readFileSync } from "node:fs";
import { render, type OutputFormat } from "./render";

const HELP = `bmm — Mermaid diagram CLI (powered by beautiful-mermaid)

USAGE
  bmm [file]              Render a .mmd file (default: ASCII)
  echo '...' | bmm        Render from stdin
  bmm --inline '...'      Render inline text

OPTIONS
  --svg                   Output SVG instead of ASCII/Unicode
  --ascii                 Use pure ASCII characters (+,-,|,>) instead of Unicode box-drawing
  --padding-x <n>         Horizontal spacing between nodes (default: 5)
  --padding-y <n>         Vertical spacing between nodes (default: 5)
  --color <mode>          Color mode: none, ansi16, ansi256, truecolor, auto (default: auto)
  --inline <text>         Pass Mermaid text directly as argument
  --help, -h              Show this help

MERMAID SYNTAX
  This tool uses standard Mermaid syntax. All diagram types supported by
  beautiful-mermaid are available: flowchart, sequence, state, class, ER, XY chart.

  Examples:
    graph LR
      A --> B
      B --> C

    graph TD
      A[Start] --> B{Decision}
      B -->|Yes| C[OK]
      B -->|No| D[Fail]

EDGE CASES & TIPS
  Edge chaining is NOT supported:
    Each edge must be declared separately.
    ✓  A --> B
       B --> C
    ✗  A --> B --> C   (only the first edge "A --> B" is recognized)

  Header line must be standalone:
    The diagram type header (graph LR, graph TD, etc.) must be on its own line.
    ✓  graph LR
         A --> B
    ✗  graph LR; A-->B   (parser error: header includes the semicolon)

  Newlines in inline mode:
    Use $'...' syntax for actual newlines in shell.
    ✓  bmm --inline $'graph LR\\nA --> B\\nB --> C'
    ✗  bmm --inline 'graph LR\\nA-->B'   (shell does not expand \\n in single quotes)

  Special characters in node IDs:
    Wrap labels in brackets. IDs themselves should be alphanumeric.
    ✓  A["Node with spaces & symbols"]
    ✗  my node-->other node

  stdin pipe:
    cat diagram.mmd | bmm
    Some editors add a trailing newline — this is harmless.
`;

interface CliArgs {
  format: OutputFormat;
  useAscii: boolean;
  paddingX?: number;
  paddingY?: number;
  colorMode?: string;
  inline?: string;
  file?: string;
  help: boolean;
}

function parseArgs(argv: string[]): CliArgs {
  const args: CliArgs = {
    format: "ascii",
    useAscii: false,
    help: false,
  };

  let i = 0;
  while (i < argv.length) {
    const arg = argv[i];

    switch (arg) {
      case "--help":
      case "-h":
        args.help = true;
        break;
      case "--svg":
        args.format = "svg";
        break;
      case "--ascii":
        args.useAscii = true;
        break;
      case "--padding-x":
        args.paddingX = Number(argv[++i]);
        break;
      case "--padding-y":
        args.paddingY = Number(argv[++i]);
        break;
      case "--color":
        args.colorMode = argv[++i];
        break;
      case "--inline":
        args.inline = argv[++i];
        break;
      case "render":
        // render is the default subcommand, just skip it
        break;
      default:
        if (!arg.startsWith("-")) {
          args.file = arg;
        } else {
          console.error(`Unknown option: ${arg}`);
          process.exit(1);
        }
    }
    i++;
  }

  return args;
}

async function readInput(args: CliArgs): Promise<string> {
  if (args.inline) return args.inline;

  if (args.file) {
    return readFileSync(args.file, "utf-8");
  }

  // stdin
  const chunks: Buffer[] = [];
  for await (const chunk of Bun.stdin.stream()) {
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf-8").trimEnd();
}

async function main() {
  const argv = process.argv.slice(2);
  const args = parseArgs(argv);

  const hasStdin = !process.stdin.isTTY;
  if (args.help || (argv.length === 0 && !hasStdin)) {
    console.log(HELP);
    process.exit(0);
  }

  const text = await readInput(args);
  if (!text) {
    console.error("Error: no input provided. Use --help for usage.");
    process.exit(1);
  }

  try {
    const output = render({
      text,
      format: args.format,
      useAscii: args.useAscii,
      paddingX: args.paddingX,
      paddingY: args.paddingY,
      colorMode: args.colorMode as any,
    });
    console.log(output);
  } catch (err) {
    console.error(
      `Error: ${err instanceof Error ? err.message : String(err)}`
    );
    process.exit(1);
  }
}

main();
