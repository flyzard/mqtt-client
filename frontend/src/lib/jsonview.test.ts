import { describe, expect, it } from "vitest";
import { jsonLines } from "./jsonview";

const text = (l: { tokens: { t: string }[] }) => l.tokens.map((x) => x.t).join("");
const changed = (lines: ReturnType<typeof jsonLines>) => lines.filter((l) => l.changed).map(text).map((s) => s.trim());

describe("jsonLines", () => {
  it("renders like JSON.stringify(v, null, 2)", () => {
    const v = { name: "x", n: -1.5, ok: true, none: null, nested: { xs: [1, { y: 2 }], empty: {} } };
    const printed = jsonLines(v).map((l) => "  ".repeat(l.indent) + text(l)).join("\n");
    expect(printed).toBe(JSON.stringify(v, null, 2));
  });

  it("tints keys, strings, numbers and keywords", () => {
    const lines = jsonLines({ k: "v", n: 1, b: false });
    expect(lines[1].tokens.map((t) => t.c)).toEqual(["key", "p", "str", "p"]);
    const classes = lines.flatMap((l) => l.tokens.map((t) => t.c));
    expect(classes).toContain("num");
    expect(classes).toContain("kw");
  });

  it("marks nothing without a previous value", () => {
    expect(changed(jsonLines({ a: 1, b: [1, 2] }))).toEqual([]);
  });

  it("marks changed and new leaves, not removed ones", () => {
    const lines = jsonLines({ a: 1, b: 2, c: { d: 3 } }, { a: 1, b: 9, z: 0 });
    expect(changed(lines)).toEqual(['"b": 2,', '"d": 3']);
  });

  it("treats a container growing from or to empty as a change", () => {
    expect(changed(jsonLines({ xs: [1] }, { xs: [] }))).toEqual(["1"]);
    expect(changed(jsonLines({ xs: [] }, { xs: [1] }))).toEqual(['"xs": []']);
    expect(changed(jsonLines({ xs: [] }, { xs: [] }))).toEqual([]);
    expect(changed(jsonLines({ xs: [] }, { xs: {} }))).toEqual(['"xs": []']);
  });

  it("is type-sensitive and compares arrays by index", () => {
    expect(changed(jsonLines({ a: "1" }, { a: 1 }))).toEqual(['"a": "1"']);
    expect(changed(jsonLines([{ a: 1 }, 2], [{ a: 1 }, 3]))).toEqual(["2"]);
    expect(changed(jsonLines({ a: 1 }, "scalar"))).toEqual(['"a": 1']);
  });
});
