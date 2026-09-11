import { describe, expect, it } from "vitest";
import { ago, bytesParts, fmtBytes, fmtRate, intervalParts, isStale, rateOf } from "./format";
import { highlight, tokenize } from "./highlight";
import { numericFields, SCALAR, series } from "./series";
import { hostOf, parseConnection } from "./broker";
import { profileColor } from "./color";
import { render } from "./payload";
import type { Msg } from "./types";

const utf8 = (s: string) => new TextEncoder().encode(s);
const msg = (id: number, text: string): Msg => ({
  id, topic: "t", qos: 0, retained: false, receivedAt: id * 1000, props: {},
  bytes: utf8(text), rendered: render(utf8(text)),
});

describe("format", () => {
  it("ago buckets", () => {
    const now = 1_000_000;
    expect(ago(now, now)).toBe("now");
    expect(ago(now - 3_000, now)).toBe("3s");
    expect(ago(now - 14 * 60_000, now)).toBe("14m");
    expect(ago(now - 2 * 3_600_000, now)).toBe("2h");
    expect(ago(now + 5000, now)).toBe("now"); // clock skew never goes negative
  });

  it("stale only when silent far beyond the usual interval", () => {
    const now = 100_000;
    expect(isStale({ avgInterval: 0, lastSeen: 0 }, now)).toBe(false); // unknown cadence
    expect(isStale({ avgInterval: 2000, lastSeen: now - 9_000 }, now)).toBe(false); // < 10s floor
    expect(isStale({ avgInterval: 2000, lastSeen: now - 11_000 }, now)).toBe(true);
    expect(isStale({ avgInterval: 60_000, lastSeen: now - 200_000 }, now)).toBe(false); // < 5x
    expect(isStale({ avgInterval: 60_000, lastSeen: now - 400_000 }, now)).toBe(true);
  });

  it("rateOf inverts the interval and drops to zero when stale", () => {
    const now = 100_000;
    expect(rateOf({ avgInterval: 2000, lastSeen: now }, now)).toBe(0.5);
    expect(rateOf({ avgInterval: 2000, lastSeen: now - 60_000 }, now)).toBe(0);
    expect(rateOf({ avgInterval: 0, lastSeen: now }, now)).toBe(0);
  });

  it("rates, intervals and sizes", () => {
    expect(fmtRate(0)).toBe("—");
    expect(fmtRate(0.5)).toBe("0.50/s");
    expect(fmtRate(2.55)).toBe("2.5/s");
    expect(fmtRate(142.4)).toBe("142/s");
    expect(intervalParts(0)).toEqual(["—"]);
    expect(intervalParts(450)).toEqual(["450", "ms"]);
    expect(intervalParts(2000)).toEqual(["2.0", "s"]);
    expect(intervalParts(90_000)).toEqual(["1.5", "m"]);
    expect(fmtBytes(11)).toBe("11 B");
    expect(fmtBytes(1536)).toBe("1.5 KB");
    expect(bytesParts(undefined)).toEqual(["—"]);
  });
});

const text = (r: Parameters<typeof highlight>[0], max?: number) => highlight(r, max).map((t) => t.t).join("");

describe("highlight", () => {
  it("compacts pretty JSON to one line and truncates", () => {
    expect(text(render(utf8('{\n  "c": 21.5\n}')))).toBe('{"c":21.5}');
    expect(text(render(utf8("a".repeat(200))), 10)).toBe("aaaaaaaaa…");
    expect(highlight(null)).toEqual([]);
    expect(highlight(render(new Uint8Array()))).toEqual([]);
  });

  it("classifies keys, strings, numbers, keywords and punctuation", () => {
    const toks = tokenize('{"c":21.5,"s":"x","ok":true,"n":null}');
    const by = (c: string) => toks.filter((t) => t.c === c).map((t) => t.t);
    expect(by("key")).toEqual(['"c"', '"s"', '"ok"', '"n"']);
    expect(by("str")).toEqual(['"x"']);
    expect(by("num")).toEqual(["21.5"]);
    expect(by("kw")).toEqual(["true", "null"]);
    expect(toks.map((t) => t.t).join("")).toBe('{"c":21.5,"s":"x","ok":true,"n":null}');
  });

  it("never loops on truncated input", () => {
    const toks = tokenize('{"c":"unterminated');
    expect(toks.map((t) => t.t).join("")).toBe('{"c":"unterminated');
  });

  it("does not tint non-JSON", () => {
    expect(highlight(render(utf8("closed")))).toEqual([{ t: "closed", c: "" }]);
    expect(highlight(render(utf8('"closed"')))).toEqual([{ t: '"closed"', c: "" }]);
  });
});

describe("series", () => {
  const msgs = [msg(3, '{"c":21.5,"meta":{"rssi":-70},"name":"k"}'), msg(2, '{"c":21.4}'), msg(1, "not json")];

  it("finds numeric fields including nested ones", () => {
    expect(numericFields(msgs)).toEqual(["c", "meta.rssi"]);
    expect(numericFields([msg(1, "42"), msg(2, "x")])).toEqual([SCALAR]);
    expect(numericFields([msg(1, "[1,2]")])).toEqual([]);
  });

  it("builds an oldest-first series over the newest n messages, skipping misses", () => {
    expect(series(msgs, "c")).toEqual([{ t: 2000, v: 21.4 }, { t: 3000, v: 21.5 }]);
    expect(series(msgs, "c", 1)).toEqual([{ t: 3000, v: 21.5 }]);
    expect(series(msgs, "meta.rssi")).toEqual([{ t: 3000, v: -70 }]);
    expect(series(msgs, "name")).toEqual([]); // present but not numeric
    expect(series([msg(1, " 7 "), msg(2, "x")], SCALAR)).toEqual([{ t: 1000, v: 7 }]);
  });
});

describe("broker", () => {
  it("parses full URLs with credentials", () => {
    expect(parseConnection("mqtts://user:p%40ss@broker.example:8883")).toEqual({
      name: "broker.example", broker: "mqtts://broker.example:8883", username: "user", password: "p@ss",
    });
  });

  it("defaults bare hosts to tcp and keeps websocket paths", () => {
    expect(parseConnection("localhost")).toMatchObject({ broker: "tcp://localhost:1883", name: "localhost", username: "" });
    expect(parseConnection("mqtts://h.example")).toMatchObject({ broker: "mqtts://h.example:8883" });
    expect(parseConnection("  10.0.0.5:1883 ")).toMatchObject({ broker: "tcp://10.0.0.5:1883" });
    expect(parseConnection("wss://h.example/mqtt?x=1")).toMatchObject({ broker: "wss://h.example/mqtt?x=1" });
  });

  it("rejects junk with a readable message", () => {
    expect(() => parseConnection("")).toThrow(/enter a broker URL/);
    expect(parseConnection("http://x")).toMatchObject({ broker: "http://x" }); // scheme is the backend's call
    expect(() => parseConnection("tcp://")).toThrow(/no host|not a valid URL/);
  });

  it("hostOf strips scheme and credentials", () => {
    expect(hostOf("mqtts://broker.example:8883")).toBe("broker.example:8883");
    expect(hostOf("garbage")).toBe("garbage");
  });
});

describe("color", () => {
  it("is stable and hex", () => {
    expect(profileColor("abc")).toBe(profileColor("abc"));
    expect(profileColor("abc")).toMatch(/^#[0-9a-f]{6}$/);
  });
});
