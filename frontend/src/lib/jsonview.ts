// Pretty-printed JSON as lines of tinted tokens, one line per scalar or
// bracket. Rendered against the previous message on the topic, so each leaf
// line knows whether its value changed; the detail pane washes those.

import type { Token } from "./highlight";

export interface Line {
  /** Nesting depth, for indentation. */
  indent: number;
  tokens: Token[];
  /** The value on this line differs from `prev`, or `prev` had no such field. */
  changed: boolean;
}

/** Marks a field `prev` does not have, as opposed to one that is `undefined`. */
const MISSING = Symbol("missing");

/** With no `prev`, a value is compared against itself and nothing is marked. */
export function jsonLines(value: unknown, prev: unknown = value): Line[] {
  const out: Line[] = [];
  emit(value, prev, 0, null, false, out);
  return out;
}

const isContainer = (v: unknown): v is object => v !== null && typeof v === "object";

function emit(v: unknown, prev: unknown, indent: number, key: string | null, comma: boolean, out: Line[]) {
  const lead: Token[] = key === null ? [] : [{ t: JSON.stringify(key), c: "key" }, { t: ": ", c: "p" }];
  const tail: Token[] = comma ? [{ t: ",", c: "p" }] : [];
  if (isContainer(v)) {
    const arr = Array.isArray(v);
    const entries = Object.entries(v);
    if (!entries.length) {
      // An empty container is a leaf: it changed if prev was not the same empty shape.
      const same = isContainer(prev) && Array.isArray(prev) === arr && !Object.keys(prev).length;
      out.push({ indent, tokens: [...lead, { t: arr ? "[]" : "{}", c: "p" }, ...tail], changed: !same });
      return;
    }
    const p = isContainer(prev) && Array.isArray(prev) === arr ? (prev as Record<string, unknown>) : null;
    out.push({ indent, tokens: [...lead, { t: arr ? "[" : "{", c: "p" }], changed: false });
    entries.forEach(([k, x], i) => {
      const px = p && k in p ? p[k] : MISSING;
      emit(x, px, indent + 1, arr ? null : k, i < entries.length - 1, out);
    });
    out.push({ indent, tokens: [{ t: arr ? "]" : "}", c: "p" }, ...tail], changed: false });
    return;
  }
  const c = typeof v === "string" ? "str" : typeof v === "number" ? "num" : "kw";
  out.push({ indent, tokens: [...lead, { t: JSON.stringify(v) ?? "null", c }, ...tail], changed: v !== prev });
}
