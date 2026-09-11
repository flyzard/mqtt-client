// Persisted UI preferences: pane sizes and whether the publish bar is open.
// Each lives under its own localStorage key; anything unreadable or out of
// range falls back to the default, so a stale value can never break the
// layout. Sizes are clamped on read as well as on write.

import { clamp } from "./format";

const PREFIX = "mqttc.";

function read<T>(key: string, valid: (v: unknown) => v is T): T | undefined {
  try {
    const raw = localStorage.getItem(PREFIX + key);
    if (raw === null) return undefined;
    const v: unknown = JSON.parse(raw);
    return valid(v) ? v : undefined;
  } catch {
    return undefined;
  }
}

function write(key: string, v: unknown) {
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify(v));
  } catch {
    // Storage unavailable: the preference simply lives for this session.
  }
}

/** One preference: a reactive value that persists on write. Never spread
 *  it, `value` is an accessor over Svelte state. */
class Pref<T> {
  #value: T;
  constructor(
    readonly key: string,
    readonly initial: T,
    valid: (v: unknown) => v is T,
    private readonly normalise: (v: T) => T = (v) => v,
  ) {
    const stored = read(key, valid);
    this.#value = $state(stored === undefined ? initial : normalise(stored));
  }
  get value() {
    return this.#value;
  }
  set value(v: T) {
    const next = this.normalise(v);
    if (next === this.#value) return; // e.g. dragging past the clamp
    this.#value = next;
    write(this.key, next);
  }
}

/** A pixel size kept within [min, max]. */
class SizePref extends Pref<number> {
  constructor(key: string, initial: number, readonly min: number, readonly max: number) {
    super(key, initial, isNum, (v) => Math.round(clamp(v, min, max)));
  }
}

const isBool = (v: unknown): v is boolean => typeof v === "boolean";
const isNum = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

export const layout = {
  /** Width of the connections pane. */
  sidebar: new SizePref("pane.sidebar", 260, 180, 480),
  /** Width of the topics pane. */
  tree: new SizePref("pane.tree", 340, 240, 800),
  /** Height of the message detail pane in the inspector. */
  detail: new SizePref("pane.detail", 240, 160, 720),
  /** Room the inspector always keeps, whatever the other two panes are dragged to. */
  inspectorMin: 420,
  /** Whether the publish bar shows its editor or only its header line. */
  publishOpen: new Pref("publish.open", true, isBool),
};
