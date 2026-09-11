const dec = new TextDecoder("utf-8", { fatal: true });
const enc = new TextEncoder();

/**
 * A classified payload. `preview` is a single line for list rows (compact
 * JSON, the first line of text, a size note for binary) and is computed
 * eagerly; `text` (pretty-printed for JSON, a hexdump for binary) is computed
 * on first access, so only the message actually opened in the inspector pays
 * for it.
 */
export interface Rendered {
  readonly kind: "json" | "text" | "hex" | "empty";
  readonly preview: string;
  readonly text: string;
  readonly value?: unknown;
}

/** Wails serialises Go []byte as base64 (or null for empty/nil). */
export function decodePayload(b64: string | null | undefined): Uint8Array {
  if (!b64) return new Uint8Array();
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** UTF-8 text to the base64 form Go's []byte expects. Safe for any Unicode. */
export function encodePayload(text: string): string {
  const bytes = enc.encode(text);
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(bin);
}

function firstLine(s: string): string {
  const nl = s.indexOf("\n");
  return nl < 0 ? s : s.slice(0, nl);
}

function lazy(kind: Rendered["kind"], preview: string, compute: () => string, value?: unknown): Rendered {
  let text: string | undefined;
  return {
    kind,
    preview,
    value,
    get text() {
      return (text ??= compute());
    },
  };
}

export function render(bytes: Uint8Array): Rendered {
  if (bytes.length === 0) return { kind: "empty", preview: "", text: "" };
  let text: string;
  try {
    text = dec.decode(bytes);
  } catch {
    return lazy("hex", `${bytes.length} bytes (binary)`, () => hexdump(bytes));
  }
  try {
    const value = JSON.parse(text);
    if (value !== null && typeof value === "object") {
      return lazy("json", JSON.stringify(value), () => JSON.stringify(value, null, 2), value);
    }
  } catch {
    // not JSON
  }
  return { kind: "text", preview: firstLine(text), text };
}

export function hexdump(b: Uint8Array): string {
  const lines: string[] = [];
  for (let i = 0; i < b.length; i += 16) {
    const chunk = b.subarray(i, i + 16);
    const hex = [...chunk].map((x) => x.toString(16).padStart(2, "0")).join(" ");
    const ascii = [...chunk].map((x) => (x >= 32 && x < 127 ? String.fromCharCode(x) : ".")).join("");
    lines.push(`${i.toString(16).padStart(8, "0")}  ${hex.padEnd(47)}  ${ascii}`);
  }
  return lines.join("\n");
}
