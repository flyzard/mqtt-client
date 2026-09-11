// A single shared 1 Hz clock so every "3s ago" label in the app ticks
// together instead of each row owning a timer. It pauses while the window is
// hidden: nothing is painted, so nothing needs re-deriving.

import { untrack } from "svelte";

export const clock = $state({ now: Date.now() });

/** Evaluate fn against the current tick. Inside $derived this reruns once a
 *  second and not on every message batch: fn's other reads are untracked. */
export function atTick<T>(fn: (now: number) => T): T {
  const now = clock.now;
  return untrack(() => fn(now));
}

setInterval(() => {
  if (!document.hidden) clock.now = Date.now();
}, 1000);
