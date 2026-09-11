// Turns whatever the user pastes into the "Add connection" box into profile
// fields: a full URL with credentials, a bare host, or host:port.

export interface ParsedConnection {
  name: string;
  broker: string;
  username: string;
  password: string;
}

const DEFAULT_PORT: Record<string, string> = { tcp: "1883", mqtt: "1883", ssl: "8883", tls: "8883", mqtts: "8883" };

/** Normalises e.g. `mqtts://user:pass@broker.example:8883`, `broker.example:1883`
 *  or `localhost` into profile fields. Only shape is checked here; the backend
 *  is the single authority on which schemes are accepted (mqtt.ParseBroker)
 *  and reports that through the save round trip. */
export function parseConnection(input: string): ParsedConnection {
  let s = input.trim();
  if (!s) throw new Error("enter a broker URL like mqtts://user:pass@host:8883");
  if (!s.includes("://")) s = "tcp://" + s;

  let u: URL;
  try {
    u = new URL(s);
  } catch {
    throw new Error(`not a valid URL: ${input.trim()}`);
  }
  const scheme = u.protocol.replace(/:$/, "").toLowerCase();
  if (!u.hostname) throw new Error("the URL has no host");

  const p = u.port || DEFAULT_PORT[scheme] || "";
  const port = p ? `:${p}` : "";
  const path = scheme === "ws" || scheme === "wss" ? u.pathname + u.search : "";
  return {
    name: u.hostname,
    broker: `${scheme}://${u.hostname}${port}${path}`,
    username: safeDecode(u.username),
    password: safeDecode(u.password),
  };
}

function safeDecode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** Host part of a stored broker URL, for compact display. */
export function hostOf(broker: string): string {
  try {
    const u = new URL(broker);
    return u.port ? `${u.hostname}:${u.port}` : u.hostname;
  } catch {
    return broker;
  }
}
