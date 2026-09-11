// Matching for the topic tree's filter box. A query with an MQTT wildcard is
// matched level by level like a topic filter (the notation MQTT users already
// think in); any other query is a case-insensitive substring match.

/**
 * Compiles a query once so the per-topic test does no string work beyond
 * the comparison itself; the tree calls it once per topic per rebuild.
 *
 * Wildcard semantics are those of a filter (`+` one level, `#` the rest),
 * except that the last level only has to be a prefix and the topic may go on
 * deeper, so `vault/+/ver` already matches `vault/2/version` while it is
 * being typed.
 */
export function compileQuery(query: string): (path: string) => boolean {
  const q = query.trim();
  if (!q) return () => true;
  if (!q.includes("#") && !q.includes("+")) {
    const needle = q.toLowerCase();
    return (path) => path.toLowerCase().includes(needle);
  }
  const f = q.split("/");
  const last = f.length - 1;
  return (path) => {
    const t = path.split("/");
    for (let i = 0; i < f.length; i++) {
      if (f[i] === "#") return true;
      if (i >= t.length) return false;
      if (f[i] === "+") continue;
      if (i === last ? !t[i].startsWith(f[i]) : f[i] !== t[i]) return false;
    }
    return true;
  };
}
