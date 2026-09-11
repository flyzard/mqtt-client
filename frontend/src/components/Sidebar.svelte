<script lang="ts">
  import { app } from "../lib/state.svelte";
  import { atTick } from "../lib/clock.svelte";
  import { canConnect, type ConnInfo, type ConnState, type Profile } from "../lib/types";
  import { hostOf, parseConnection } from "../lib/broker";
  import { profileColor } from "../lib/color";
  import { fmtRate, rateOf } from "../lib/format";

  let url = $state("");
  let editing = $state<Profile | null>(null);

  /** Status dot and subline colour per connection state. */
  const busy = { dot: "bg-warn animate-pulse", cls: "text-warn" };
  const look: Record<ConnState, { dot: string; cls: string }> = {
    connected: { dot: "bg-ok", cls: "text-muted" },
    connecting: busy,
    reconnecting: busy,
    failed: { dot: "bg-bad", cls: "text-bad" },
    disconnected: { dot: "bg-ink/25", cls: "text-muted" },
  };

  // Messages per second per connection from the backend's per-topic
  // intervals, summed once per clock tick rather than on every topics batch.
  const rates = $derived(
    atTick((now) => {
      const out = new Map<string, number>();
      for (const p of app.profiles) {
        let r = 0;
        for (const s of app.topics.get(p.id)?.values() ?? []) r += rateOf(s, now);
        out.set(p.id, r);
      }
      return out;
    }),
  );

  function subline(p: Profile, { state, error }: ConnInfo): string {
    switch (state) {
      case "connected": {
        const topics = app.topics.get(p.id)?.size ?? 0;
        return `${fmtRate(rates.get(p.id) ?? 0)} · ${topics} topic${topics === 1 ? "" : "s"}`;
      }
      case "connecting":
        return "connecting…";
      case "reconnecting":
        return error ? `reconnecting · ${error}` : "reconnecting…";
      case "failed":
        return error ?? "failed";
      default:
        return hostOf(p.broker);
    }
  }

  async function add() {
    let parsed;
    try {
      parsed = parseConnection(url);
    } catch (e) {
      app.fail(e);
      return;
    }
    const stored = await app.saveProfile({ id: "", clientId: "", hasPassword: false, cleanStart: true, insecure: false, ...parsed });
    if (!stored) return;
    url = "";
    app.selectProfile(stored.id);
    await app.connect(stored.id);
  }

  function open(p: Profile) {
    app.selectProfile(p.id);
    if (canConnect(app.stateOf(p.id))) void app.connect(p.id);
  }

  async function save() {
    if (editing && (await app.saveProfile(editing))) editing = null;
  }

  async function remove(p: Profile) {
    if (confirm(`Delete "${p.name || hostOf(p.broker)}"?`)) await app.deleteProfile(p.id);
  }
</script>

<div class="h-full min-h-0 flex flex-col">
  <div class="flex-1 min-h-0 overflow-y-auto">
    {#each app.profiles as p (p.id)}
      {@const info = app.conn.get(p.id) ?? { state: "disconnected" as ConnState }}
      {@const selected = app.selectedProfile === p.id}
      {@const offline = canConnect(info.state)}
      {@const sub = subline(p, info)}
      <div
        class="group relative accent border-b border-line transition-colors {selected ? 'sel' : 'hover:bg-ink/3'}"
        style={offline ? "" : `--accent: ${profileColor(p.id)}`}
      >
        <button
          class="w-full text-left pl-4 pr-3 py-2"
          onclick={() => app.selectProfile(p.id)}
          ondblclick={() => open(p)}
          title={offline ? "Double-click to connect" : ""}
        >
          <div class="flex items-center gap-2">
            <!-- Re-keyed on every batch so the ring replays. -->
            {#key app.activity.get(p.id)}
              <span class="relative size-2 shrink-0 rounded-full {look[info.state].dot} {info.state === 'connected' ? 'ping' : ''}"></span>
            {/key}
            <span class="truncate font-semibold {offline ? 'text-ink/60' : 'text-ink'}">{p.name || hostOf(p.broker)}</span>
          </div>
          <div class="pl-4 text-[11px] truncate {look[info.state].cls}" title={sub}>{sub}</div>
        </button>
        <div class="card absolute right-1.5 top-1.5 hidden group-hover:flex items-center gap-0.5 px-1 py-0.5 text-[11px]">
          <button class="btn-flat" onclick={() => app.toggleConnection(p.id)}>
            {offline ? "connect" : "disconnect"}
          </button>
          <button class="btn-flat px-1.5" title="Edit" onclick={() => (editing = { ...p, password: "" })}>✎</button>
          <button class="btn-flat px-1.5 hover:text-bad" title="Delete" onclick={() => remove(p)}>×</button>
        </div>
      </div>

      {#if editing?.id === p.id}
        <form class="px-3 py-3 flex flex-col gap-2 border-b border-line bg-paper" onsubmit={(e) => { e.preventDefault(); save(); }}>
          <input class="inp text-xs" placeholder="Name" bind:value={editing.name} />
          <input class="inp text-xs font-mono" placeholder="tcp://host:1883" bind:value={editing.broker} required spellcheck="false" />
          <input class="inp text-xs" placeholder="Username" bind:value={editing.username} autocomplete="off" />
          <input class="inp text-xs" type="password" autocomplete="new-password"
                 placeholder={editing.hasPassword ? "Password (unchanged)" : "Password"} bind:value={editing.password} />
          <label class="flex gap-1.5 items-center text-xs text-muted"><input type="checkbox" bind:checked={editing.cleanStart} /> clean start</label>
          <label class="flex gap-1.5 items-center text-xs text-muted"><input type="checkbox" bind:checked={editing.insecure} /> skip TLS verify</label>
          <div class="flex gap-2 pt-1">
            <button class="btn-primary flex-1 text-xs" type="submit">Save</button>
            <button class="btn text-xs" type="button" onclick={() => (editing = null)}>Cancel</button>
          </div>
        </form>
      {/if}
    {/each}

    {#if app.ready && !app.profiles.length}
      <div class="empty">
        <div class="empty-title">No connections yet</div>
        Paste a broker URL below. A bare host is enough to get started.
      </div>
    {/if}
  </div>

  <div class="border-t border-line px-3 py-3">
    <div class="text-xs text-ink mb-1.5">Add connection</div>
    <form onsubmit={(e) => { e.preventDefault(); add(); }}>
      <input
        class="inp w-full font-mono text-xs"
        placeholder="mqtts://user:pass@host:8883"
        bind:value={url}
        spellcheck="false"
        autocomplete="off"
      />
    </form>
    <div class="text-[10px] text-muted/80 mt-1.5">Enter to add and connect. A bare host defaults to tcp on 1883.</div>
  </div>
</div>
