<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";
  import { app } from "../lib/state.svelte";
  import { clock } from "../lib/clock.svelte";
  import { ago, fmtCount, isStale } from "../lib/format";
  import { canConnect, type TopicStats } from "../lib/types";
  import Payload from "./Payload.svelte";

  type Node = {
    name: string;
    path: string;
    stats?: TopicStats;
    count: number; // messages in this subtree
    children: Node[]; // sorted
  };

  let filter = $state("");
  let byActivity = $state(false);
  let newFilter = $state("");
  let filterEl = $state<HTMLInputElement>();
  const collapsed = new SvelteSet<string>();

  const profileId = $derived(app.selectedProfile);
  const connState = $derived(profileId ? app.stateOf(profileId) : "disconnected");
  const stats = $derived(profileId ? app.topics.get(profileId) : undefined);
  const subs = $derived(profileId ? (app.subs.get(profileId) ?? []) : []);
  const query = $derived(filter.trim().toLowerCase());

  // "/" focuses the filter unless the user is already typing somewhere.
  function hotkey(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    const typing = !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
    if (e.key === "/" && !typing) {
      e.preventDefault();
      filterEl?.focus();
    }
  }

  // ---- tree -------------------------------------------------------------------
  const collator = new Intl.Collator(undefined, { numeric: true });
  const byName = (a: Node, b: Node) => collator.compare(a.name, b.name);
  const byCount = (a: Node, b: Node) => b.count - a.count || byName(a, b);

  type Building = Omit<Node, "children"> & { children: Map<string, Building> };

  const tree = $derived.by(() => {
    const root: Building = { name: "", path: "", count: 0, children: new Map() };
    for (const [path, s] of stats ?? []) {
      if (query && !path.toLowerCase().includes(query)) continue;
      let cur = root;
      for (const part of path.split("/")) {
        let n = cur.children.get(part);
        if (!n) {
          n = { name: part, path: cur.path ? `${cur.path}/${part}` : part, count: 0, children: new Map() };
          cur.children.set(part, n);
        }
        n.count += s.count;
        cur = n;
      }
      cur.stats = s;
    }
    return finish(root);
  });

  // Sort once per rebuild rather than once per render of every open node.
  function finish(b: Building): Node {
    const children = [...b.children.values()].map(finish).sort(byActivity ? byCount : byName);
    return { name: b.name, path: b.path, stats: b.stats, count: b.count, children };
  }

  function toggle(path: string) {
    if (!collapsed.delete(path)) collapsed.add(path);
  }
  function activate(c: Node) {
    if (c.stats) void app.selectTopic(c.path);
    else toggle(c.path);
  }
  async function addSub() {
    const f = newFilter.trim();
    if (!f) return;
    await app.subscribe(f, 0);
    newFilter = "";
  }
</script>

{#snippet row(c: Node, depth: number)}
  {@const s = c.stats}
  {@const open = query !== "" || !collapsed.has(c.path)}
  {@const selected = app.selectedTopic === c.path}
  {@const stale = s ? isStale(s, clock.now) : false}
  <div
    class="flex items-center gap-1.5 h-7 pr-2 cursor-pointer transition-colors duration-300
           accent {selected ? 'accent-on sel' : app.flashing.has(c.path) ? 'accent-off flash' : 'accent-off hover:bg-ink/3'}
           {stale && !selected ? 'opacity-50' : ''}"
    style="padding-left: {5 + depth * 14}px"
    role="button"
    tabindex="0"
    onclick={() => activate(c)}
    onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); activate(c); } }}
  >
    {#if c.children.length}
      <button class="w-3 shrink-0 text-muted text-[10px] outline-none" tabindex="-1" onclick={(e) => { e.stopPropagation(); toggle(c.path); }}>
        {open ? "▾" : "▸"}
      </button>
    {:else}<span class="w-3 shrink-0"></span>{/if}
    <span class="truncate shrink-0 max-w-[55%] {s ? 'text-ink' : 'text-ink/80'}">{c.name}</span>
    <span class="flex-1 min-w-0 truncate text-right font-mono text-[11px] text-muted">
      <Payload value={s?.last ?? null} max={60} />
    </span>
    {#if s?.retained}<span class="badge" title="retained">R</span>{/if}
    {#if s}
      <span class="w-8 shrink-0 text-right text-[11px] tabular-nums text-muted"
            title="{fmtCount(s.count)} messages{stale ? ' · quiet for longer than usual' : ''}">
        {ago(s.lastSeen, clock.now)}
      </span>
    {:else}
      <span class="shrink-0 text-right text-[11px] tabular-nums text-muted" title="messages in this branch">{fmtCount(c.count)}</span>
    {/if}
  </div>
  {#if open}
    {#each c.children as k (k.path)}{@render row(k, depth + 1)}{/each}
  {/if}
{/snippet}

<svelte:window onkeydown={hotkey} />

<div class="h-full min-h-0 flex flex-col">
  <div class="flex items-center gap-2 px-2 py-2">
    <input
      bind:this={filterEl}
      class="inp flex-1 min-w-0 text-xs"
      placeholder="filter   /"
      bind:value={filter}
      spellcheck="false"
      onkeydown={(e) => { if (e.key === "Escape") { filter = ""; e.currentTarget.blur(); } }}
    />
    <button class="chip {byActivity ? 'chip-on' : ''}" title="Sort by message count" onclick={() => (byActivity = !byActivity)}>activity</button>
  </div>

  <div class="flex-1 min-h-0 overflow-y-auto py-1">
    {#if !profileId}
      <div class="px-4 py-8 text-xs text-muted leading-relaxed">
        <div class="text-ink font-semibold text-sm mb-1">No connection selected</div>
        Pick one on the left. Double-click connects.
      </div>
    {:else if canConnect(connState)}
      <div class="px-4 py-8 text-xs text-muted flex flex-col gap-3 items-start">
        <div class="text-ink font-semibold text-sm">Not connected</div>
        <button class="btn-primary text-xs" onclick={() => app.connect(profileId)}>Connect</button>
      </div>
    {:else if tree.children.length === 0}
      <div class="px-4 py-8 text-xs text-muted leading-relaxed">
        {#if filter}
          Nothing matches the filter.
        {:else}
          <div class="text-ink font-semibold text-sm mb-1 flex items-center gap-2">
            <span class="inline-block size-2 rounded-full bg-ok animate-pulse"></span> Listening
          </div>
          Topics appear here as messages arrive. Press <kbd>/</kbd> to filter them.
        {/if}
      </div>
    {:else}
      {#each tree.children as k (k.path)}{@render row(k, 0)}{/each}
    {/if}
  </div>

  {#if profileId}
    <div class="border-t border-line px-3 py-1.5 text-[11px] text-muted flex flex-wrap items-center gap-x-1.5 gap-y-1">
      <span>via</span>
      {#each subs as s (s.filter)}
        <span class="inline-flex items-center gap-1 font-mono text-ink" title="QoS {s.qos}">
          {s.filter}
          <button class="text-muted/70 hover:text-bad" title="Unsubscribe" onclick={() => app.unsubscribe(s.filter)}>×</button>
        </span>
        <span class="text-ink/30">·</span>
      {/each}
      <form class="inline-flex" onsubmit={(e) => { e.preventDefault(); addSub(); }}>
        <input class="bg-transparent outline-none font-mono w-24 text-ink placeholder:text-muted/70" placeholder="+ filter" bind:value={newFilter} spellcheck="false" />
      </form>
    </div>
  {/if}
</div>
