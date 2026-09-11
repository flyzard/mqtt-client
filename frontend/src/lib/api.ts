// api.ts is the ONLY module that imports the generated Wails bindings or the
// Wails runtime. It converts binding shapes into the domain types in types.ts:
// nullable Go slices become arrays, base64 payloads become decoded + rendered
// bytes, ISO timestamps become epoch ms, and every failure is an ApiError
// naming the operation.

import { Events } from "@wailsio/runtime";
import { ProfileService, SessionService } from "../../bindings/mqttc/internal/services";
import type * as mqtt from "../../bindings/mqttc/internal/mqtt";
import type * as services from "../../bindings/mqttc/internal/services";
import { decodePayload, encodePayload, render, type Rendered } from "./payload";
import type {
  AppEvent, ConnState, Msg, Profile, PublishInput, QoS, SessionSnap, Snapshot, Subscription,
  TopicStats, WatchResult,
} from "./types";

export class ApiError extends Error {
  constructor(public readonly op: string, cause: unknown) {
    super(`${op}: ${messageOf(cause)}`, { cause });
    this.name = "ApiError";
  }
}

/** Best-effort message for anything thrown, including the Wails runtime's
 *  plain `{message}` objects. */
export function messageOf(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (e && typeof e === "object" && "message" in e) return String((e as { message: unknown }).message);
  return String(e);
}

async function call<T>(op: string, p: Promise<T>): Promise<T> {
  try {
    return await p;
  } catch (e) {
    throw new ApiError(op, e);
  }
}

// ---- conversions ------------------------------------------------------------

const arr = <T>(a: T[] | null | undefined): T[] => a ?? [];
const ms = (iso: string) => Date.parse(iso);

function toMsg(m: mqtt.Message): Msg {
  const bytes = decodePayload(m.payload);
  const p = m.props ?? {};
  return {
    id: m.id,
    topic: m.topic,
    qos: m.qos as number as QoS,
    retained: m.retained,
    receivedAt: ms(m.receivedAt),
    props: {
      contentType: p.contentType || undefined,
      responseTopic: p.responseTopic || undefined,
      correlationData: p.correlationData ? decodePayload(p.correlationData) : undefined,
      messageExpiry: p.messageExpiry ?? undefined,
      user: p.user ? Object.fromEntries(Object.entries(p.user).filter(([, v]) => v !== undefined)) as Record<string, string> : undefined,
    },
    bytes,
    rendered: render(bytes),
  };
}

const toSub = (s: mqtt.Subscription): Subscription => ({ filter: s.filter, qos: s.qos as number as QoS });
const toState = (s: mqtt.ConnState): ConnState => s as string as ConnState;
// Every dirty topic arrives up to 20 times a second; decoding and classifying
// its preview is deferred until a row actually renders it.
function toStats(t: mqtt.TopicStats): TopicStats {
  const b64 = t.lastPayload;
  let last: Rendered | null | undefined;
  return {
    path: t.path,
    count: t.count,
    bytes: t.bytes,
    retained: t.retained,
    lastSeen: ms(t.lastSeen),
    avgInterval: t.avgInterval * 1000,
    get last() {
      return (last ??= b64 ? render(decodePayload(b64)) : null);
    },
  };
}

function toProfile(p: services.Profile): Profile {
  const { password: _omit, ...rest } = p;
  return rest;
}

function toSession(s: mqtt.SessionSnapshot): SessionSnap {
  return {
    profileId: s.profileId,
    seq: s.seq,
    state: toState(s.state),
    error: s.error || undefined,
    subscriptions: arr(s.subscriptions).map(toSub),
    topics: arr(s.topics).map(toStats),
  };
}

// ---- RPC --------------------------------------------------------------------

export const api = {
  async snapshot(): Promise<Snapshot> {
    const s = await call("snapshot", SessionService.Snapshot());
    return { profiles: arr(s.profiles).map(toProfile), sessions: arr(s.sessions).map(toSession) };
  },

  async saveProfile(p: Profile): Promise<Profile> {
    return toProfile(await call("save profile", ProfileService.Save({ ...p, password: p.password ?? "" })));
  },

  deleteProfile: (id: string) => call("delete profile", ProfileService.Delete(id)),
  connect: (profileId: string) => call("connect", SessionService.Connect(profileId)),
  disconnect: (profileId: string) => call("disconnect", SessionService.Disconnect(profileId)),

  subscribe: (profileId: string, sub: Subscription) =>
    call("subscribe", SessionService.Subscribe(profileId, { filter: sub.filter, qos: sub.qos as mqtt.QoS })),
  unsubscribe: (profileId: string, filter: string) => call("unsubscribe", SessionService.Unsubscribe(profileId, filter)),

  publish: (profileId: string, p: PublishInput) =>
    call("publish", SessionService.Publish(profileId, {
      topic: p.topic, payload: encodePayload(p.text), qos: p.qos as mqtt.QoS, retain: p.retain, props: {},
    })),

  async watch(profileId: string, topic: string, limit: number): Promise<WatchResult> {
    const w = await call("watch", SessionService.Watch(profileId, topic, limit));
    return { profileId: w.profileId, topic: w.topic, messages: arr(w.messages).map(toMsg) };
  },
  unwatch: (profileId: string, topic: string) => call("unwatch", SessionService.Unwatch(profileId, topic)),
  clear: (profileId: string, topic: string) => call("clear", SessionService.Clear(profileId, topic)),
};

export type Api = typeof api;

// ---- events -----------------------------------------------------------------

/** Subscribes to every backend event. Returns an unsubscribe function. */
export function listen(cb: (e: AppEvent) => void): () => void {
  const offs = [
    Events.On("state:changed", ({ data: d }) =>
      cb({ kind: "state", profileId: d.profileId, seq: d.seq, state: toState(d.state), error: d.error || undefined })),
    Events.On("messages:batch", ({ data: d }) =>
      cb({ kind: "messages", profileId: d.profileId, seq: d.seq, messages: arr(d.messages).map(toMsg), dropped: d.dropped })),
    Events.On("topics:changed", ({ data: d }) =>
      cb({ kind: "topics", profileId: d.profileId, seq: d.seq, upserts: arr(d.upserts).map(toStats), removed: arr(d.removed) })),
    Events.On("subscriptions:changed", ({ data: d }) =>
      cb({ kind: "subscriptions", profileId: d.profileId, seq: d.seq, active: arr(d.active).map(toSub) })),
  ];
  return () => offs.forEach((off) => off());
}
