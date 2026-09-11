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

export function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
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
export function fmtRate(perSec: number): string {
  if (!(perSec > 0)) return "—";
  if (perSec >= 10) return `${Math.round(perSec)}/s`;
  if (perSec >= 1) return `${perSec.toFixed(1)}/s`;
  return `${perSec.toFixed(2)}/s`;
}

/** An interval in ms: "450ms", "2.0s", "1.5m", "—" for unknown. */
export function fmtInterval(ms: number): string {
  if (!(ms > 0)) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)}m`;
  return `${(ms / 3_600_000).toFixed(1)}h`;
}

const TIME = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });

/** "10:42:07" */
export function fmtTime(epochMs: number): string {
  return TIME.format(epochMs);
}
