// Domain types used by the store and components. They are deliberately
// independent of the generated Wails bindings: api.ts is the only module that
// imports bindings, and it converts to these shapes (arrays never null,
// payloads already decoded and classified, timestamps as epoch ms).

import type { Rendered } from "./payload";

export type ConnState = "disconnected" | "connecting" | "connected" | "reconnecting" | "failed";

/** Mirrors ConnState.Live() in Go: the session owns a broker connection. */
export const isLive = (s: ConnState) => s === "connecting" || s === "connected" || s === "reconnecting";
/** Mirrors the Go transition table: states from which Connect is legal. */
export const canConnect = (s: ConnState) => !isLive(s);

export type QoS = 0 | 1 | 2;
export const QOS_LEVELS: readonly QoS[] = [0, 1, 2];

export interface Profile {
  id: string;
  name: string;
  broker: string;
  clientId: string;
  username: string;
  /** Write-only: set to change the stored password, leave empty to keep it. */
  password?: string;
  hasPassword: boolean;
  cleanStart: boolean;
  insecure: boolean;
}

export interface Subscription {
  filter: string;
  qos: QoS;
}

export interface TopicStats {
  path: string;
  count: number;
  bytes: number;
  lastSeen: number; // epoch ms
  retained: boolean;
  /** Newest payload (truncated by the backend), already classified; null before the first message. */
  last: Rendered | null;
  /** Smoothed gap between messages in ms; 0 until the topic has seen two. */
  avgInterval: number;
}

export interface Props {
  contentType?: string;
  responseTopic?: string;
  correlationData?: Uint8Array;
  messageExpiry?: number;
  user?: Record<string, string>;
}

export interface Msg {
  id: number;
  topic: string;
  qos: QoS;
  retained: boolean;
  receivedAt: number; // epoch ms
  props: Props;
  bytes: Uint8Array;
  rendered: Rendered;
}

export interface ConnInfo {
  state: ConnState;
  error?: string;
}

export interface SessionSnap {
  profileId: string;
  seq: number;
  state: ConnState;
  error?: string;
  subscriptions: Subscription[];
  topics: TopicStats[];
}

export interface Snapshot {
  profiles: Profile[];
  sessions: SessionSnap[];
}

export interface WatchResult {
  profileId: string;
  topic: string;
  messages: Msg[];
}

export interface PublishInput {
  topic: string;
  text: string;
  qos: QoS;
  retain: boolean;
}

/** Backend events, one variant per Wails event name. Every one carries the
 *  per-session seq used to reconcile it against snapshots. */
export type AppEvent =
  | { kind: "state"; profileId: string; seq: number; state: ConnState; error?: string }
  | { kind: "messages"; profileId: string; seq: number; messages: Msg[]; dropped: number }
  | { kind: "topics"; profileId: string; seq: number; upserts: TopicStats[]; removed: string[] }
  | { kind: "subscriptions"; profileId: string; seq: number; active: Subscription[] };
