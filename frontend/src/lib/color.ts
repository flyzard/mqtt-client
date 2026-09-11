// A stable accent colour per connection, so the same broker is always the
// same colour in the sidebar bar, the publish bar and the connection tag.

// Muted, warm tones that sit on the cream surface without shouting.
const PALETTE = [
  "#c2613f", // terracotta
  "#b8892b", // ochre
  "#6f8f3d", // olive
  "#3b8b7e", // teal
  "#4f6fa8", // slate blue
  "#8a5a9e", // plum
  "#b85c7a", // rose
  "#a5502c", // rust
];

/** Deterministic colour for a profile id. */
export function profileColor(id: string): string {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return PALETTE[h % PALETTE.length];
}
