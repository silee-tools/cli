#!/usr/bin/env bun

import {
  renderMermaidASCII,
  renderMermaidSVG,
  type AsciiRenderOptions,
  type RenderOptions,
} from "beautiful-mermaid";

export type OutputFormat = "ascii" | "svg";

export interface RenderInput {
  text: string;
  format: OutputFormat;
  useAscii?: boolean;
  paddingX?: number;
  paddingY?: number;
  colorMode?: AsciiRenderOptions["colorMode"];
}

export function render(input: RenderInput): string {
  if (input.format === "svg") {
    return renderMermaidSVG(input.text);
  }

  const options: AsciiRenderOptions = {};
  if (input.useAscii) options.useAscii = true;
  if (input.paddingX !== undefined) options.paddingX = input.paddingX;
  if (input.paddingY !== undefined) options.paddingY = input.paddingY;
  if (input.colorMode !== undefined) options.colorMode = input.colorMode;

  return renderMermaidASCII(input.text, options);
}
