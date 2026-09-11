<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { atTick } from "../lib/clock.svelte";
  import { layout } from "../lib/prefs.svelte";
  import { canConnect, type ConnInfo, type ConnState, type Profile } from "../lib/types";
  import { hostOf, parseConnection } from "../lib/broker";
  import { profileColor } from "../lib/color";
  import { fmtRate, rateOf } from "../lib/format";
  import Chevron from "./Chevron.svelte";

  let url = $state("");
  let urlEl = $state<HTMLInputElement>();
  let editing = $state<Profile | null>(null);
  /** Profile whose delete is awaiting confirmation. Inline, because the
   *  webview shows no native confirm() dialog. */
  let deleting = $state<string | null>(null);
  /** The add form is a single row until asked for, except on first run. */
  let adding = $state(false);
  const showAdd = $derived(adding || (app.ready && !app.profiles.length));
  const open = $derived(layout.sidebarOpen.value);

  async function openAdd() {
    adding = true;
    await tick();
    urlEl?.focus();
  }
  function closeAdd() {
    adding = false;
    url = "";
  }

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

  const info = (p: Profile): ConnInfo => app.conn.get(p.id) ?? { state: "disconnected" };
  const label = (p: Profile) => p.name || hostOf(p.broker);

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
    closeAdd();
    app.selectProfile(stored.id);
    await app.connect(stored.id);
  }

  function pick(p: Profile) {
    app.selectProfile(p.id);
    if (canConnect(info(p).state)) void app.connect(p.id);
  }

  async function save() {
    if (editing && (await app.saveProfile(editing))) editing = null;
  }

  async function remove(id: string) {
    deleting = null;
    await app.deleteProfile(id);
  }
</script>

{#if !open}
  <!-- Rail: one dot per connection, the selected one ringed in its colour. -->
  <div class="h-full min-h-0 flex flex-col items-center">
    <div class="flex-1 min-h-0 overflow-y-auto flex flex-col items-center gap-1 py-2">
      {#each app.profiles as p (p.id)}
        {@const i = info(p)}
        {@const selected = app.selectedProfile === p.id}
        <button
          class="grid place-items-center size-8 rounded-md transition-colors {selected ? 'sel' : 'hover:bg-ink/3'}"
          style="--accent: {profileColor(p.id)}"
          title="{label(p)} · {subline(p, i)}{canConnect(i.state) ? ' · double-click to connect' : ''}"
          onclick={() => app.selectProfile(p.id)}
          ondblclick={() => pick(p)}
        >
          {#key app.activity.get(p.id)}
            <span class="relative size-2.5 rounded-full {look[i.state].dot} {i.state === 'connected' ? 'ping' : ''} {selected ? 'ring-2 ring-offset-2 ring-offset-cream ring-(--accent)' : ''}"></span>
          {/key}
        </button>
      {/each}
    </div>
    <button class="btn-flat w-full h-9 grid place-items-center border-t border-line rounded-none" title="Show connections"
            onclick={() => (layout.sidebarOpen.value = true)}><Chevron dir="right" /></button>
  </div>
{:else}
  <div class="h-full min-h-0 flex flex-col">
    <div class="flex-1 min-h-0 overflow-y-auto">
      {#each app.profiles as p (p.id)}
        {@const i = info(p)}
        {@const selected = app.selectedProfile === p.id}
        {@const offline = canConnect(i.state)}
        {@const sub = subline(p, i)}
        <div
          class="group relative accent border-b border-line transition-colors {selected ? 'sel' : 'hover:bg-ink/3'}"
          style={offline ? "" : `--accent: ${profileColor(p.id)}`}
        >
          <button
            class="w-full text-left pl-4 pr-3 py-2"
            onclick={() => app.selectProfile(p.id)}
            ondblclick={() => pick(p)}
            title={offline ? "Double-click to connect" : ""}
          >
            <div class="flex items-center gap-2">
              <!-- Re-keyed on every batch so the ring replays. -->
              {#key app.activity.get(p.id)}
                <span class="relative size-2 shrink-0 rounded-full {look[i.state].dot} {i.state === 'connected' ? 'ping' : ''}"></span>
              {/key}
              <span class="truncate font-semibold {offline ? 'text-ink/60' : 'text-ink'}">{label(p)}</span>
            </div>
            <div class="pl-4 text-[11px] truncate {look[i.state].cls}" title={sub}>{sub}</div>
          </button>
          {#if deleting !== p.id}
            <div class="card absolute right-1.5 top-1.5 hidden group-hover:flex items-center gap-0.5 px-1 py-0.5 text-[11px]">
              <button class="btn-flat" onclick={() => app.toggleConnection(p.id)}>
                {offline ? "connect" : "disconnect"}
              </button>
              <button class="btn-flat px-1.5" title="Edit" onclick={() => (editing = { ...p, password: "" })}>✎</button>
              <button class="btn-flat px-1.5 hover:text-bad" title="Delete" onclick={() => (deleting = p.id)}>×</button>
            </div>
          {/if}
        </div>

        {#if deleting === p.id}
          <div class="flex items-center gap-2 px-4 py-2 border-b border-line bg-paper text-xs">
            <span class="flex-1 truncate text-ink">Delete <span class="font-semibold">{label(p)}</span>?</span>
            <button class="btn text-xs border-bad/60 text-bad hover:bg-bad/8" onclick={() => remove(p.id)}>Delete</button>
            <button class="btn-flat text-xs" onclick={() => (deleting = null)}>cancel</button>
          </div>
        {/if}

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

    <div class="border-t border-line">
      {#if showAdd}
        <form class="px-3 py-3" onsubmit={(e) => { e.preventDefault(); add(); }}>
          <div class="flex items-center justify-between mb-1.5">
            <span class="text-xs text-ink">Add connection</span>
            {#if app.profiles.length}
              <button class="btn-flat text-[11px] -mr-2" type="button" onclick={closeAdd}>cancel</button>
            {/if}
          </div>
          <input
            bind:this={urlEl}
            class="inp w-full font-mono text-xs"
            placeholder="mqtts://user:pass@host:8883"
            bind:value={url}
            spellcheck="false"
            autocomplete="off"
            onkeydown={(e) => { if (e.key === "Escape" && app.profiles.length) closeAdd(); }}
          />
          <div class="text-[10px] text-muted/80 mt-1.5">Enter to add and connect. A bare host defaults to tcp on 1883.</div>
        </form>
      {:else}
        <div class="flex items-stretch">
          <button class="flex-1 text-left px-4 py-2.5 text-xs text-muted hover:text-ink hover:bg-ink/3 transition-colors" onclick={openAdd}>
            + Add connection
          </button>
          <button class="btn-flat rounded-none px-3 grid place-items-center border-l border-line" title="Fold to a rail"
                  onclick={() => (layout.sidebarOpen.value = false)}><Chevron dir="left" /></button>
        </div>
      {/if}
    </div>
  </div>
{/if}
