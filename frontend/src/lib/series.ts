// Numeric extraction for the inspector sparkline: which fields of a topic's
// recent messages are numbers, and the time series of one of them.

import type { Msg } from "./types";

/** Field name used when the payload itself is a bare number. */
export const SCALAR = "";

/** Numeric fields seen in the newest `sample` messages, in first-seen order.
 *  Nested objects are flattened to dotted paths up to two levels deep. */
export function numericFields(msgs: Msg[], sample = 20): string[] {
  const fields = new Set<string>();
  for (const m of msgs.slice(0, sample)) {
    const r = m.rendered;
    if (r.kind === "json") collect(r.value, "", fields, 0);
    else if (r.kind === "text" && isNumeric(r.text)) fields.add(SCALAR);
  }
  return [...fields];
}

function isNumeric(s: string): boolean {
  const t = s.trim();
  return t !== "" && Number.isFinite(Number(t));
}

function collect(v: unknown, prefix: string, out: Set<string>, depth: number) {
  if (!v || typeof v !== "object" || Array.isArray(v)) return;
  for (const [k, x] of Object.entries(v)) {
    const path = prefix ? `${prefix}.${k}` : k;
    if (typeof x === "number" && Number.isFinite(x)) out.add(path);
    else if (depth < 2 && x && typeof x === "object" && !Array.isArray(x)) collect(x, path, out, depth + 1);
  }
}

/** The numeric value at `path` (empty for a bare-number payload), or null when absent. */
function valueAt(m: Msg, path: string[]): number | null {
  const r = m.rendered;
  if (path.length === 0) return r.kind === "text" && isNumeric(r.text) ? Number(r.text) : null;
  if (r.kind !== "json") return null;
  let cur: unknown = r.value;
  for (const part of path) {
    if (!cur || typeof cur !== "object") return null;
    cur = (cur as Record<string, unknown>)[part];
  }
  return typeof cur === "number" && Number.isFinite(cur) ? cur : null;
}

export interface Point {
  /** Epoch ms the message arrived. */
  t: number;
  v: number;
}

/** Values of `field` across the newest `n` messages, oldest first, skipping
 *  messages that lack it. `msgs` is newest-first as in the store. */
export function series(msgs: Msg[], field: string, n = 60): Point[] {
  const path = field === SCALAR ? [] : field.split(".");
  const out: Point[] = [];
  for (const m of msgs.slice(0, n)) {
    const v = valueAt(m, path);
    if (v !== null) out.push({ t: m.receivedAt, v });
  }
  return out.reverse();
}
