// One-line payload rendering for dense rows: a compact form of the payload
// plus a tiny JSON tokenizer so keys, strings and numbers can be tinted.

import type { Rendered } from "./payload";

export type TokenClass = "key" | "str" | "num" | "kw" | "p" | "";

export interface Token {
  t: string;
  c: TokenClass;
}

/** A payload's one-line preview, truncated to `max` characters. */
function compact(r: Rendered | null, max = 120): string {
  const s = r?.preview ?? "";
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

// Every alternative is guaranteed to consume at least one character, so the
// loop always terminates even on truncated or malformed input.
const TOKEN =
  /"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\btrue\b|\bfalse\b|\bnull\b|[{}[\]:,]|\s+|[^\s"{}[\]:,]+|[\s\S]/g;

/** Splits a compact JSON string into classified tokens. Non-JSON input comes
 *  back as best-effort tokens; nothing is thrown. */
export function tokenize(src: string): Token[] {
  const out: Token[] = [];
  TOKEN.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = TOKEN.exec(src))) {
    const t = m[0];
    let c: TokenClass = "";
    const ch = t[0];
    if (ch === '"') c = src[TOKEN.lastIndex] === ":" ? "key" : "str"; // compact JSON has no whitespace
    else if (ch === "-" || (ch >= "0" && ch <= "9")) c = "num";
    else if (t === "true" || t === "false" || t === "null") c = "kw";
    else if ("{}[]:,".includes(ch)) c = "p";
    out.push({ t, c });
  }
  return out;
}

/** Tokens for a payload's compact form. Only JSON is tinted. */
export function highlight(r: Rendered | null, max = 120): Token[] {
  const s = compact(r, max);
  if (!s) return [];
  return r?.kind === "json" ? tokenize(s) : [{ t: s, c: "" }];
}

/** Tailwind classes per token class. */
export const TOKEN_CLASS: Record<TokenClass, string> = {
  key: "text-syn-key",
  str: "text-syn-str",
  num: "text-syn-num",
  kw: "text-syn-kw",
  p: "text-muted",
  "": "",
};
