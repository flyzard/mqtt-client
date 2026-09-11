import { describe, expect, it } from "vitest";
import { compileQuery } from "./topics";

const matches = (path: string, query: string) => compileQuery(query)(path);

describe("topic query matching", () => {
  it("uses substring matching for plain queries", () => {
    expect(matches("vault/2/Version", "vers")).toBe(true);
    expect(matches("vault/2/version", "access")).toBe(false);
    expect(matches("anything", "   ")).toBe(true);
  });

  it("treats wildcard queries as filters with a prefix on the last level", () => {
    expect(matches("vault/2/version", "vault/+/ver")).toBe(true);
    expect(matches("vault/2/version/extra", "vault/+/version")).toBe(true);
    expect(matches("vault/2/commands", "vault/+/ver")).toBe(false);
    expect(matches("access/218", "vault/#")).toBe(false);
    expect(matches("vault", "vault/#")).toBe(true);
    expect(matches("vault", "vault/+")).toBe(false);
    expect(matches("a/b", "+/b")).toBe(true);
    expect(matches("x/b/c", "a/+/c")).toBe(false);
  });
});
