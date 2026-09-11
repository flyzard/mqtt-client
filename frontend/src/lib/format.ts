// Small, pure formatting helpers shared by the panes. Everything takes
// numbers in (epoch ms, bytes, per-second rates) and returns short strings
// sized for dense rows.

import type { TopicStats } from "./types";

/** "now", "3s", "14m", "2h", "3d". */
export function ago(lastSeen: number, now: number): string {
  const s = Math.max(0, (now - lastSeen) / 1000);
  if (s < 1) return "now";
  if (s < 60) return `${s | 0}s`;
  if (s < 3600) return `${(s / 60) | 0}m`;
  if (s < 86400) return `${(s / 3600) | 0}h`;
  return `${(s / 86400) | 0}d`;
}

/** A topic is stale when it has been silent for much longer than its usual gap. */
export function isStale(s: Pick<TopicStats, "avgInterval" | "lastSeen">, now: number): boolean {
  if (!s.avgInterval) return false;
  return now - s.lastSeen > Math.max(10_000, s.avgInterval * 5);
}

/** `n` kept within [min, max]. */
export const clamp = (n: number, min: number, max: number) => Math.min(max, Math.max(min, n));

/** A number and its unit, kept apart so the UI can style them differently. */
export type Quantity = [num: string, unit?: string];
const NONE: Quantity = ["—"];

/** A size in bytes: "11 B", "1.5 KB", "—" when there is none. */
export function bytesParts(n: number | undefined): Quantity {
  if (n === undefined) return NONE;
  if (n < 1024) return [String(n), "B"];
  if (n < 1024 * 1024) return [(n / 1024).toFixed(1), "KB"];
  return [(n / 1024 / 1024).toFixed(1), "MB"];
}
export function fmtBytes(n: number): string {
  return bytesParts(n).join(" ");
}

const COUNT = new Intl.NumberFormat();
export function fmtCount(n: number): string {
  return COUNT.format(n);
}

/** Messages per second implied by a topic's smoothed interval; 0 once the
 *  topic has gone quiet, so a dead sensor does not keep reporting a rate. */
export function rateOf(s: Pick<TopicStats, "avgInterval" | "lastSeen">, now: number): number {
  return s.avgInterval && !isStale(s, now) ? 1000 / s.avgInterval : 0;
}

/** Messages per second: "142/s", "2.5/s", "0.05/s", "—" for zero. */
export function rateParts(perSec: number): Quantity {
  if (!(perSec > 0)) return NONE;
  if (perSec >= 10) return [String(Math.round(perSec)), "/s"];
  if (perSec >= 1) return [perSec.toFixed(1), "/s"];
  return [perSec.toFixed(2), "/s"];
}
export function fmtRate(perSec: number): string {
  return rateParts(perSec).join("");
}

/** An interval in ms: "450ms", "2.0s", "1.5m", "—" for unknown. */
export function intervalParts(ms: number): Quantity {
  if (!(ms > 0)) return NONE;
  if (ms < 1000) return [String(Math.round(ms)), "ms"];
  if (ms < 60_000) return [(ms / 1000).toFixed(1), "s"];
  if (ms < 3_600_000) return [(ms / 60_000).toFixed(1), "m"];
  return [(ms / 3_600_000).toFixed(1), "h"];
}

/** A chart value: integers as-is, otherwise four significant digits. */
export function fmtNum(v: number): string {
  return Number.isInteger(v) ? String(v) : v.toPrecision(4).replace(/\.?0+$/, "");
}

const TIME = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });

/** "10:42:07" */
export function fmtTime(epochMs: number): string {
  return TIME.format(epochMs);
}
