import { beforeEach, describe, expect, it, vi } from "vitest";
import { AppState, MAX_MESSAGES } from "./state.svelte";
import type { Api } from "./api";
import type { AppEvent, ConnState, Msg, Profile, SessionSnap, Snapshot, TopicStats } from "./types";

// ---- fixtures ---------------------------------------------------------------

const profile = (id: string, over: Partial<Profile> = {}): Profile => ({
  id, name: id, broker: "tcp://x:1883", clientId: "c", username: "", hasPassword: false,
  cleanStart: true, insecure: false, ...over,
});

const stats = (path: string, count = 1): TopicStats =>
  ({ path, count, bytes: 0, lastSeen: 0, retained: false, last: null, avgInterval: 0 });

const msg = (id: number, topic = "t"): Msg => ({
  id, topic, qos: 0, retained: false, receivedAt: 0, props: {},
  bytes: new Uint8Array(), rendered: { kind: "empty", preview: "", text: "" },
});

const session = (profileId: string, seq: number, state: ConnState, over: Partial<SessionSnap> = {}): SessionSnap =>
  ({ profileId, seq, state, subscriptions: [], topics: [], ...over });

const ev = {
  state: (profileId: string, seq: number, state: ConnState, error?: string): AppEvent =>
    ({ kind: "state", profileId, seq, state, error }),
  messages: (profileId: string, seq: number, messages: Msg[], dropped = 0): AppEvent =>
    ({ kind: "messages", profileId, seq, messages, dropped }),
  topics: (profileId: string, seq: number, upserts: TopicStats[], removed: string[] = []): AppEvent =>
    ({ kind: "topics", profileId, seq, upserts, removed }),
};

type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void };
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void, reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)));
  return { promise, resolve, reject };
}

function fakeApi(over: Partial<Api> = {}): Api {
  return {
    snapshot: vi.fn(async (): Promise<Snapshot> => ({ profiles: [], sessions: [] })),
    saveProfile: vi.fn(async (p: Profile) => ({ ...p, id: p.id || "new", password: undefined })),
    deleteProfile: vi.fn(async () => {}),
    connect: vi.fn(async () => {}),
    disconnect: vi.fn(async () => {}),
    subscribe: vi.fn(async () => {}),
    unsubscribe: vi.fn(async () => {}),
    publish: vi.fn(async () => {}),
    watch: vi.fn(async (profileId: string, topic: string) => ({ profileId, topic, messages: [] })),
    unwatch: vi.fn(async () => {}),
    clear: vi.fn(async () => {}),
    ...over,
  };
}

/** A listen() stand-in that lets the test push events. */
function fakeListen() {
  let cb: ((e: AppEvent) => void) | null = null;
  const off = vi.fn(() => (cb = null));
  return {
    listen: (fn: (e: AppEvent) => void) => ((cb = fn), off),
    emit: (e: AppEvent) => cb!(e),
    off,
  };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

// ---- tests ------------------------------------------------------------------

describe("init and hydration", () => {
  it("applies the snapshot and marks ready", async () => {
    const api = fakeApi({
      snapshot: vi.fn(async (): Promise<Snapshot> => ({
        profiles: [profile("a"), profile("b")],
        sessions: [session("a", 10, "connected", { subscriptions: [{ filter: "#", qos: 0 }], topics: [stats("x/y")] })],
      })),
    });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();

    expect(app.ready).toBe(true);
    expect(app.profiles.map((p) => p.id)).toEqual(["a", "b"]);
    expect(app.conn.get("a")).toEqual({ state: "connected", error: undefined });
    expect(app.conn.get("b")).toBeUndefined();
    expect(app.subs.get("a")).toEqual([{ filter: "#", qos: 0 }]);
    expect(app.topics.get("a")?.get("x/y")?.count).toBe(1);
  });

  it("buffers events during the snapshot and drops those the snapshot already covers", async () => {
    const snap = deferred<Snapshot>();
    const api = fakeApi({ snapshot: vi.fn(() => snap.promise) });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    const init = app.init();

    // Arrive while the snapshot RPC is in flight: seq 9 is stale, 11 is newer.
    l.emit(ev.state("a", 9, "connecting"));
    l.emit(ev.topics("a", 11, [stats("new")]));
    expect(app.conn.size).toBe(0); // nothing applied yet

    snap.resolve({
      profiles: [profile("a")],
      sessions: [session("a", 10, "connected", { topics: [stats("old")] })],
    });
    await init;

    expect(app.conn.get("a")?.state).toBe("connected"); // seq 9 dropped
    expect([...app.topics.get("a")!.keys()].sort()).toEqual(["new", "old"]); // seq 11 applied on top
  });

  it("surfaces a failed snapshot as a toast and still becomes ready", async () => {
    const api = fakeApi({ snapshot: vi.fn(async () => { throw new Error("boom"); }) });
    const app = new AppState(api, fakeListen().listen);
    await app.init();
    expect(app.ready).toBe(true);
    expect(app.toasts.map((t) => t.text)).toEqual(["boom"]);
  });

  it("destroy unsubscribes", async () => {
    const l = fakeListen();
    const app = new AppState(fakeApi(), l.listen);
    await app.init();
    app.destroy();
    expect(l.off).toHaveBeenCalledOnce();
  });
});

describe("reducer", () => {
  let app: AppState;
  let l: ReturnType<typeof fakeListen>;
  beforeEach(async () => {
    l = fakeListen();
    app = new AppState(fakeApi({ snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })) }), l.listen);
    await app.init();
  });

  it("tracks connection state and errors", () => {
    l.emit(ev.state("a", 1, "connecting"));
    l.emit(ev.state("a", 2, "reconnecting", "dial tcp: refused"));
    expect(app.conn.get("a")).toEqual({ state: "reconnecting", error: "dial tcp: refused" });
  });

  it("ignores out-of-order or duplicate seq", () => {
    l.emit(ev.state("a", 5, "connected"));
    l.emit(ev.state("a", 5, "failed"));
    l.emit(ev.state("a", 3, "failed"));
    expect(app.conn.get("a")?.state).toBe("connected");
  });

  it("upserts and removes topics per profile", () => {
    l.emit(ev.topics("a", 1, [stats("x", 1), stats("y", 1)]));
    l.emit(ev.topics("a", 2, [stats("x", 2)], ["y"]));
    l.emit(ev.topics("b", 1, [stats("z")]));
    expect([...app.topics.get("a")!.keys()]).toEqual(["x"]);
    expect(app.topics.get("a")!.get("x")!.count).toBe(2);
    expect([...app.topics.get("b")!.keys()]).toEqual(["z"]);
  });

  it("ignores message batches when nothing is watched", () => {
    l.emit(ev.messages("a", 1, [msg(1)], 3));
    expect(app.messages).toEqual([]);
    expect(app.dropped).toBe(0);
  });
});

describe("watching a topic", () => {
  it("shows history newest-first and prepends live messages for the watched topic only", async () => {
    const api = fakeApi({
      snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })),
      watch: vi.fn(async () => ({ profileId: "a", topic: "t", messages: [msg(3), msg(2), msg(1)] })),
    });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    await app.selectTopic("t");

    expect(api.watch).toHaveBeenCalledWith("a", "t", expect.any(Number));
    expect(app.messages.map((m) => m.id)).toEqual([3, 2, 1]);

    l.emit(ev.messages("a", 6, [msg(4), msg(5, "other"), msg(6)], 2));
    expect(app.messages.map((m) => m.id)).toEqual([6, 4, 3, 2, 1]);
    expect(app.dropped).toBe(2);

    l.emit(ev.messages("b", 1, [msg(99)]));
    expect(app.messages.map((m) => m.id)).toEqual([6, 4, 3, 2, 1]);
  });

  it("merges by id when a live batch beats the watch response", async () => {
    const w = deferred<Awaited<ReturnType<Api["watch"]>>>();
    const api = fakeApi({
      snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })),
      watch: vi.fn(() => w.promise),
    });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    const sel = app.selectTopic("t");

    l.emit(ev.messages("a", 1, [msg(4), msg(5)]));
    w.resolve({ profileId: "a", topic: "t", messages: [msg(5), msg(4), msg(3)] });
    await sel;

    expect(app.messages.map((m) => m.id)).toEqual([5, 4, 3]);
    l.emit(ev.messages("a", 2, [msg(5), msg(6)])); // 5 is a duplicate
    expect(app.messages.map((m) => m.id)).toEqual([6, 5, 4, 3]);
  });

  it("drops a stale watch response after switching topics", async () => {
    const first = deferred<Awaited<ReturnType<Api["watch"]>>>();
    const api = fakeApi({
      snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })),
      watch: vi.fn()
        .mockImplementationOnce(() => first.promise)
        .mockImplementationOnce(async () => ({ profileId: "a", topic: "u", messages: [msg(10, "u")] })),
    });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    const sel1 = app.selectTopic("t");
    await app.selectTopic("u");
    first.resolve({ profileId: "a", topic: "t", messages: [msg(1, "t")] });
    await sel1;

    expect(app.selectedTopic).toBe("u");
    expect(app.messages.map((m) => m.id)).toEqual([10]);
    expect(api.unwatch).toHaveBeenCalledWith("a", "t");
  });

  it("caps the message list", async () => {
    const api = fakeApi({ snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })) });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    await app.selectTopic("t");
    const many = Array.from({ length: MAX_MESSAGES + 50 }, (_, i) => msg(i + 1));
    l.emit(ev.messages("a", 1, many));
    expect(app.messages).toHaveLength(MAX_MESSAGES);
    expect(app.messages[0].id).toBe(MAX_MESSAGES + 50);
  });

  it("clears messages when the watched topic is cleared on the backend", async () => {
    const api = fakeApi({ snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })) });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    await app.selectTopic("t");
    l.emit(ev.messages("a", 1, [msg(1)], 4));
    l.emit(ev.topics("a", 2, [], ["t"]));
    expect(app.messages).toEqual([]);
    expect(app.dropped).toBe(0);
  });

  it("clearTopic only asks the backend; the resulting event clears the list", async () => {
    const api = fakeApi({ snapshot: vi.fn(async () => ({ profiles: [profile("a")], sessions: [] })) });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();
    app.selectProfile("a");
    await app.selectTopic("t");
    l.emit(ev.messages("a", 1, [msg(1)]));
    await app.clearTopic();
    expect(api.clear).toHaveBeenCalledWith("a", "t");
    expect(app.messages).toHaveLength(1);
    l.emit(ev.topics("a", 2, [], ["t"]));
    expect(app.messages).toEqual([]);
  });
});

describe("commands", () => {
  it("routes connection toggling by state and reports failures as toasts", async () => {
    const api = fakeApi({ connect: vi.fn(async () => { throw new Error("connect: already connecting"); }) });
    const l = fakeListen();
    const app = new AppState(api, l.listen);
    await app.init();

    await app.toggleConnection("a");
    expect(api.connect).toHaveBeenCalledWith("a");
    expect(app.toasts.map((t) => t.text)).toEqual(["connect: already connecting"]);

    l.emit(ev.state("a", 1, "connected"));
    await app.toggleConnection("a");
    expect(api.disconnect).toHaveBeenCalledWith("a");

    app.dismiss(app.toasts[0].id);
    expect(app.toasts).toEqual([]);
  });

  it("saves and deletes profiles, cleaning up per-profile state", async () => {
    const api = fakeApi({
      snapshot: vi.fn(async (): Promise<Snapshot> => ({
        profiles: [profile("a")],
        sessions: [session("a", 1, "connected", { topics: [stats("t")] })],
      })),
    });
    const app = new AppState(api, fakeListen().listen);
    await app.init();

    await app.saveProfile(profile("", { name: "fresh" }));
    expect(app.profiles.map((p) => p.id)).toEqual(["a", "new"]);
    await app.saveProfile(profile("a", { name: "renamed" }));
    expect(app.profiles.find((p) => p.id === "a")?.name).toBe("renamed");

    app.selectProfile("a");
    await app.selectTopic("t");
    await app.deleteProfile("a");
    expect(api.deleteProfile).toHaveBeenCalledWith("a");
    expect(app.profiles.map((p) => p.id)).toEqual(["new"]);
    expect(app.conn.has("a")).toBe(false);
    expect(app.topics.has("a")).toBe(false);
    expect(app.selectedProfile).toBeNull();
    expect(app.selectedTopic).toBeNull();
  });

  it("refuses to publish without a selected profile", async () => {
    const api = fakeApi();
    const app = new AppState(api, fakeListen().listen);
    await app.init();
    app.publish({ topic: "t", text: "x", qos: 0, retain: false });
    await tick();
    expect(api.publish).not.toHaveBeenCalled();
    expect(app.toasts).toHaveLength(1);
  });
});
