import { SvelteMap } from "svelte/reactivity";
import { api, listen, messageOf, type Api } from "./api";
import {
  canConnect, type AppEvent, type ConnInfo, type ConnState, type Msg, type Profile, type PublishInput,
  type QoS, type Snapshot, type Subscription, type TopicStats,
} from "./types";

/** Max messages kept for the watched topic. */
export const MAX_MESSAGES = 1000;
/** History requested when a topic is first watched. */
export const HISTORY_LIMIT = 200;
const TOAST_MS = 6000;
/** How long a topic row stays highlighted after receiving a message. */
export const FLASH_MS = 450;

export interface Toast {
  id: number;
  text: string;
}

interface Watching {
  profileId: string;
  topic: string;
}

/** Newest-first union of two message lists, deduplicated by id and capped.
 *  Msg objects are immutable, so this is the only place the list is shaped. */
export function mergeMessages(existing: Msg[], incoming: Msg[]): Msg[] {
  if (!incoming.length) return existing;
  // Live batches arrive id-ascending and, in steady state, entirely newer
  // than what is shown: a plain prepend, no Set and no sort. (Watch history
  // is newest-first and takes the general path.)
  const ascending = incoming[incoming.length - 1].id >= incoming[0].id;
  if (ascending && existing.length && incoming[0].id > existing[0].id) {
    return [...incoming].reverse().concat(existing).slice(0, MAX_MESSAGES);
  }
  const seen = new Set(existing.map((m) => m.id));
  const fresh = incoming.filter((m) => !seen.has(m.id));
  if (!fresh.length) return existing;
  return [...fresh, ...existing].sort((a, b) => b.id - a.id).slice(0, MAX_MESSAGES);
}

/**
 * AppState is the single store. Components read it and call its command
 * methods; nothing else talks to the backend.
 *
 * Consistency model:
 *  - Every backend event carries a per-session `seq`. A snapshot carries the
 *    seq it was taken at. Events with seq <= the last snapshot seq are already
 *    reflected in that snapshot and are dropped.
 *  - While a snapshot is in flight, events are buffered and replayed after it
 *    is applied, so nothing is lost or applied to stale state.
 *  - Topic history (Watch) and the live stream are reconciled purely by the
 *    monotonic message id, so it does not matter which arrives first.
 */
export class AppState {
  profiles = $state<Profile[]>([]);
  selectedProfile = $state<string | null>(null);
  ready = $state(false);

  conn = new SvelteMap<string, ConnInfo>();
  subs = new SvelteMap<string, Subscription[]>();
  topics = new SvelteMap<string, SvelteMap<string, TopicStats>>(); // profileId -> path -> stats
  /** Messages for the watched profile+topic, newest first. Raw: Msg is immutable
   *  and the array is always replaced wholesale, so no deep proxying is needed. */
  messages = $state.raw<Msg[]>([]);
  dropped = $state(0);
  toasts = $state<Toast[]>([]);
  /** Topics of the selected profile that received a message within FLASH_MS,
   *  mapped to the time the highlight expires. Kept in deadline order. */
  flashing = new SvelteMap<string, number>();
  private flashTimer?: ReturnType<typeof setTimeout>;
  /** Time of the last batch per profile; the sidebar pulses on each change. */
  activity = new SvelteMap<string, number>();

  private watching = $state.raw<Watching | null>(null);
  private lastSeq = new Map<string, number>();
  private buffered: AppEvent[] | null = null;
  private off?: () => void;
  private nextToast = 1;

  constructor(private readonly rpc: Api = api, private readonly listenFn: typeof listen = listen) {}

  /** The topic currently watched, if any. Derived from `watching` so the two
   *  can never disagree. */
  get selectedTopic(): string | null {
    return this.watching?.topic ?? null;
  }

  /** Connection state for a profile; profiles without a session are disconnected. */
  stateOf(profileId: string): ConnState {
    return this.conn.get(profileId)?.state ?? "disconnected";
  }

  // ---- lifecycle ------------------------------------------------------------

  async init() {
    this.buffered = [];
    this.off = this.listenFn((e) => (this.buffered ? this.buffered.push(e) : this.apply(e)));
    try {
      this.applySnapshot(await this.rpc.snapshot());
    } catch (e) {
      this.fail(e);
    }
    const queued = this.buffered;
    this.buffered = null;
    for (const e of queued) this.apply(e);
    this.ready = true;
  }

  destroy() {
    this.off?.();
    this.off = undefined;
    clearTimeout(this.flashTimer);
  }

  // ---- reducer ----------------------------------------------------------------

  applySnapshot(snap: Snapshot) {
    this.profiles = snap.profiles;
    for (const s of snap.sessions) {
      this.conn.set(s.profileId, { state: s.state, error: s.error });
      this.subs.set(s.profileId, s.subscriptions);
      this.topics.set(s.profileId, new SvelteMap(s.topics.map((t) => [t.path, t])));
      this.lastSeq.set(s.profileId, s.seq);
    }
  }

  apply(e: AppEvent) {
    if (e.seq <= (this.lastSeq.get(e.profileId) ?? 0)) return; // already in a snapshot
    this.lastSeq.set(e.profileId, e.seq);
    const w = this.watching;
    const watched = w?.profileId === e.profileId ? w : null;
    switch (e.kind) {
      case "state":
        this.conn.set(e.profileId, { state: e.state, error: e.error });
        break;
      case "subscriptions":
        this.subs.set(e.profileId, e.active);
        break;
      case "topics": {
        let t = this.topics.get(e.profileId);
        if (!t) {
          t = new SvelteMap();
          this.topics.set(e.profileId, t);
        }
        for (const u of e.upserts) t.set(u.path, u);
        for (const r of e.removed) t.delete(r);
        if (e.upserts.length && this.ready) this.activity.set(e.profileId, Date.now());
        if (e.profileId === this.selectedProfile && this.ready) {
          this.flash(e.upserts);
          for (const r of e.removed) this.flashing.delete(r);
        }
        if (watched && e.removed.includes(watched.topic)) {
          // History for the watched topic was cleared, by us or by anyone.
          this.messages = [];
          this.dropped = 0;
        }
        break;
      }
      case "messages":
        if (!watched) break;
        this.messages = mergeMessages(this.messages, e.messages.filter((m) => m.topic === watched.topic));
        this.dropped += e.dropped;
        break;
    }
  }

  // ---- flash ------------------------------------------------------------------

  /** Marks topics as just-changed. Re-inserting keeps the map in deadline
   *  order, so one sweep timer retires expired entries from the front. */
  private flash(changed: TopicStats[]) {
    const until = Date.now() + FLASH_MS;
    for (const { path } of changed) {
      this.flashing.delete(path);
      this.flashing.set(path, until);
    }
    this.flashTimer ??= setTimeout(() => this.sweepFlashes(), FLASH_MS);
  }

  private sweepFlashes() {
    const now = Date.now();
    let next: number | undefined;
    for (const [path, until] of this.flashing) {
      if (until > now) {
        next = until;
        break;
      }
      this.flashing.delete(path);
    }
    this.flashTimer = next === undefined ? undefined : setTimeout(() => this.sweepFlashes(), Math.max(50, next - now));
  }

  // ---- selection --------------------------------------------------------------

  selectProfile(id: string) {
    if (this.selectedProfile === id) return;
    this.selectedProfile = id;
    this.flashing.clear();
    this.clearWatch();
  }

  async selectTopic(path: string) {
    const profileId = this.selectedProfile;
    if (!profileId) return;
    if (this.watching?.profileId === profileId && this.watching.topic === path) return;

    this.clearWatch();
    const w: Watching = { profileId, topic: path };
    this.watching = w;

    const res = await this.run(() => this.rpc.watch(profileId, path, HISTORY_LIMIT));
    if (!res || this.watching !== w) return; // failed or superseded meanwhile
    this.messages = mergeMessages(this.messages, res.messages); // live batches may have landed first
  }

  private clearWatch() {
    const prev = this.watching;
    this.watching = null;
    this.messages = [];
    this.dropped = 0;
    if (prev) void this.run(() => this.rpc.unwatch(prev.profileId, prev.topic));
  }

  // ---- commands ---------------------------------------------------------------

  connect(id: string) {
    return this.run(() => this.rpc.connect(id));
  }

  disconnect(id: string) {
    return this.run(() => this.rpc.disconnect(id));
  }

  toggleConnection(id: string) {
    return canConnect(this.stateOf(id)) ? this.connect(id) : this.disconnect(id);
  }

  async saveProfile(p: Profile) {
    const stored = await this.run(() => this.rpc.saveProfile(p));
    if (!stored) return undefined;
    const i = this.profiles.findIndex((q) => q.id === stored.id);
    this.profiles = i < 0 ? [...this.profiles, stored] : this.profiles.with(i, stored);
    return stored;
  }

  async deleteProfile(id: string) {
    try {
      await this.rpc.deleteProfile(id);
    } catch (e) {
      return this.fail(e);
    }
    this.profiles = this.profiles.filter((p) => p.id !== id);
    this.conn.delete(id);
    this.subs.delete(id);
    this.topics.delete(id);
    this.lastSeq.delete(id);
    if (this.selectedProfile === id) {
      this.selectedProfile = null;
      this.clearWatch();
    }
  }

  subscribe(filter: string, qos: QoS) {
    const id = this.selectedProfile;
    if (!id || !filter) return;
    return this.run(() => this.rpc.subscribe(id, { filter, qos }));
  }

  unsubscribe(filter: string) {
    const id = this.selectedProfile;
    if (!id) return;
    return this.run(() => this.rpc.unsubscribe(id, filter));
  }

  /** Resolves true once the broker accepted the publish, undefined on failure. */
  publish(p: PublishInput) {
    const id = this.selectedProfile;
    if (!id) return this.fail(new Error("select a connection first"));
    return this.run(async () => {
      await this.rpc.publish(id, p);
      return true as const;
    });
  }

  /** Clears the watched topic's history. The UI updates when the backend's
   *  topics:changed event names the topic, like any other clear. */
  clearTopic() {
    const w = this.watching;
    if (!w) return;
    return this.run(() => this.rpc.clear(w.profileId, w.topic));
  }

  // ---- errors -----------------------------------------------------------------

  private async run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    try {
      return await fn();
    } catch (e) {
      this.fail(e);
      return undefined;
    }
  }

  fail(e: unknown) {
    const id = this.nextToast++;
    this.toasts = [...this.toasts, { id, text: messageOf(e) }];
    setTimeout(() => this.dismiss(id), TOAST_MS);
  }

  dismiss(id: number) {
    this.toasts = this.toasts.filter((t) => t.id !== id);
  }
}

export const app = new AppState();
