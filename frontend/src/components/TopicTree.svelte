<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";
  import { app } from "../lib/state.svelte";
  import { clock } from "../lib/clock.svelte";
  import { reveal } from "../lib/dom";
  import { ago, fmtCount, isStale } from "../lib/format";
  import { compileQuery } from "../lib/topics";
  import { canConnect, type TopicStats } from "../lib/types";
  import Chevron from "./Chevron.svelte";
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
  let treeEl = $state<HTMLDivElement>();
  const collapsed = new SvelteSet<string>();
  /** Row the arrow keys act on. Separate from the watched topic so branches
   *  can be walked and folded without selecting anything. */
  let cursor = $state<string | null>(null);

  const profileId = $derived(app.selectedProfile);
  const connState = $derived(profileId ? app.stateOf(profileId) : "disconnected");
  const stats = $derived(profileId ? app.topics.get(profileId) : undefined);
  const subs = $derived(profileId ? (app.subs.get(profileId) ?? []) : []);
  const query = $derived(filter.trim());
  const matches = $derived(compileQuery(query));
  // A filter shows everything it matched, so folding is suspended while one is set.
  const canFold = $derived(query === "");

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

  // The root plus every branch path (for fold/unfold all), collected in the
  // same pass that sorts the children.
  const built = $derived.by(() => {
    const root: Building = { name: "", path: "", count: 0, children: new Map() };
    for (const [path, s] of stats ?? []) {
      if (!matches(path)) continue;
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
    const branches: string[] = [];
    return { tree: finish(root, branches), branches };
  });
  const tree = $derived(built.tree);
  const branches = $derived(built.branches);

  // Sort once per rebuild rather than once per render of every open node.
  function finish(b: Building, branches: string[]): Node {
    const children = [...b.children.values()].map((k) => finish(k, branches)).sort(byActivity ? byCount : byName);
    if (b.path && children.length) branches.push(b.path);
    return { name: b.name, path: b.path, stats: b.stats, count: b.count, children };
  }

  const isOpen = (path: string) => !canFold || !collapsed.has(path);
  const allFolded = $derived(branches.length > 0 && branches.every((p) => collapsed.has(p)));

  /** Rows in display order: what the keyboard walks. Only read from key
   *  handlers, so it is rebuilt at most once per keypress. */
  const visible = $derived.by(() => {
    const out: Node[] = [];
    const walk = (n: Node) => {
      for (const k of n.children) {
        out.push(k);
        if (k.children.length && isOpen(k.path)) walk(k);
      }
    };
    walk(tree);
    return out;
  });
  const rowAt = (path: string | null) => (path === null ? -1 : visible.findIndex((n) => n.path === path));

  function toggle(path: string) {
    if (!collapsed.delete(path)) collapsed.add(path);
  }
  function foldAll(fold: boolean) {
    if (fold) for (const p of branches) collapsed.add(p);
    else collapsed.clear();
  }
  function activate(c: Node) {
    cursor = c.path;
    if (c.stats) void app.selectTopic(c.path);
    else toggle(c.path);
  }

  // ---- keyboard -----------------------------------------------------------------
  function moveTo(c: Node) {
    cursor = c.path;
    // Walking onto a topic shows it; walking onto a branch only moves the cursor.
    if (c.stats) void app.selectTopic(c.path);
    void reveal(treeEl, "data-path", c.path);
  }

  const HANDLED = new Set(["ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight", "Home", "End", "Enter", " "]);

  function keys(e: KeyboardEvent) {
    if (!HANDLED.has(e.key) || !visible.length) return;
    e.preventDefault();
    const i = rowAt(cursor ?? app.selectedTopic);
    const cur = i >= 0 ? visible[i] : undefined;
    const go = (j: number) => moveTo(visible[Math.max(0, Math.min(visible.length - 1, j))]);
    switch (e.key) {
      case "ArrowDown": return go(i + 1);
      case "ArrowUp": return go(i < 0 ? 0 : i - 1);
      case "Home": return go(0);
      case "End": return go(visible.length - 1);
      case "ArrowRight":
        if (!cur) return go(0);
        if (!cur.children.length) return;
        if (!isOpen(cur.path)) return toggle(cur.path);
        return go(i + 1);
      case "ArrowLeft": {
        if (!cur) return;
        if (cur.children.length && isOpen(cur.path) && canFold) return toggle(cur.path);
        const parent = rowAt(cur.path.slice(0, cur.path.lastIndexOf("/")));
        if (parent >= 0) moveTo(visible[parent]);
        return;
      }
      default: // Enter, Space
        if (cur) activate(cur);
    }
  }

  // From the filter box: Enter opens the first matching topic, ↓ steps into the tree.
  function filterKeys(e: KeyboardEvent & { currentTarget: HTMLInputElement }) {
    if (e.key === "Escape") {
      filter = "";
      e.currentTarget.blur();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      treeEl?.focus();
      const c = visible[Math.max(0, rowAt(cursor))];
      if (c) moveTo(c);
    } else if (e.key === "Enter") {
      e.preventDefault();
      const first = visible.find((n) => n.stats);
      if (first) {
        treeEl?.focus();
        moveTo(first);
      }
    }
  }

  async function addSub() {
    const f = newFilter.trim();
    if (!f) return;
    await app.subscribe(f, 0);
    newFilter = "";
  }
</script>

<!-- A branch is a group: its header row sticks to the top of the pane (offset
     by one row per ancestor, so the whole path stays readable while scrolling)
     and its children hang from an indent guide. Leaves are plain rows. -->
{#snippet row(c: Node, depth: number)}
  {@const s = c.stats}
  {@const open = isOpen(c.path)}
  {@const selected = app.selectedTopic === c.path}
  {@const stale = s ? isStale(s, clock.now) : false}
  {#if s}
    <!-- svelte-ignore a11y_click_events_have_key_events -- keys are handled once, on the role="tree" container -->
    <div
      class={[
        "tree-row accent",
        selected ? "accent-on sel" : ["accent-off", app.flashing.has(c.path) ? "flash" : "hover:bg-ink/3"],
        cursor === c.path && !selected && "tree-cursor",
        stale && !selected && "opacity-50",
      ]}
      style="scroll-margin-top: {depth * 28}px"
      role="treeitem"
      aria-level={depth + 1}
      aria-selected={selected}
      tabindex="-1"
      data-path={c.path}
      onclick={() => activate(c)}
    >
      <span class="w-4 shrink-0"></span>
      <span class="truncate shrink-0 max-w-[55%] text-ink">{c.name}</span>
      <span class="flex-1 min-w-0 truncate text-right font-mono text-[11px] text-muted">
        <Payload value={s.last} max={60} />
      </span>
      {#if s.retained}<span class="badge" title="retained">R</span>{/if}
      <span class="w-8 shrink-0 text-right text-[11px] tabular-nums text-muted"
            title="{fmtCount(s.count)} messages{stale ? ' · quiet for longer than usual' : ''}">
        {ago(s.lastSeen, clock.now)}
      </span>
    </div>
  {:else}
    <div role="group" aria-label={c.name}>
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div
        class={["tree-row tree-head accent accent-off", cursor === c.path && "tree-cursor", !open && "hover:bg-ink/3"]}
        style="top: {depth * 28}px; z-index: {20 - depth}; scroll-margin-top: {depth * 28}px"
        role="treeitem"
        aria-level={depth + 1}
        aria-selected={false}
        aria-expanded={open}
        tabindex="-1"
        data-path={c.path}
        onclick={() => activate(c)}
      >
        <span class="w-4 shrink-0 grid place-items-center text-muted"><Chevron {open} /></span>
        <span class="truncate font-semibold text-ink/80">{c.name}</span>
        <span class="tree-count" title="messages in this branch">{fmtCount(c.count)}</span>
        <span class="flex-1"></span>
        <span class="text-[11px] text-muted/70 tabular-nums" title="topics in this branch">{c.children.length}</span>
      </div>
      {#if open}
        <div class="tree-kids">
          {#each c.children as k (k.path)}{@render row(k, depth + 1)}{/each}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

<svelte:window onkeydown={hotkey} />

<div class="h-full min-h-0 flex flex-col">
  <div class="flex items-center gap-2 px-2 py-2">
    <input
      bind:this={filterEl}
      class="inp flex-1 min-w-0 text-xs"
      placeholder="filter   /"
      title="Substring, or an MQTT filter like vault/+/version"
      bind:value={filter}
      spellcheck="false"
      onkeydown={filterKeys}
    />
    <button class="chip chip-toggle" class:chip-on={byActivity} title="Sort by message count" onclick={() => (byActivity = !byActivity)}>activity</button>
    <button class="btn-flat text-[11px] px-1.5"
            disabled={!branches.length || !canFold}
            title={!canFold ? "Clear the filter to fold branches" : allFolded ? "Expand every branch" : "Collapse every branch"}
            onclick={() => foldAll(!allFolded)}>
      {allFolded ? "unfold" : "fold"}
    </button>
  </div>

  <div bind:this={treeEl} class="tree flex-1 min-h-0 overflow-y-auto outline-none"
       role="tree" aria-label="topics" tabindex="0" onkeydown={keys}>
    {#if !profileId}
      <div class="empty">
        <div class="empty-title">No connection selected</div>
        Pick one on the left. Double-click connects.
      </div>
    {:else if canConnect(connState)}
      <div class="empty flex flex-col gap-2 items-start">
        <div class="empty-title">Not connected</div>
        <button class="btn-primary text-xs" onclick={() => app.connect(profileId)}>Connect</button>
      </div>
    {:else if tree.children.length === 0}
      <div class="empty">
        {#if filter}
          Nothing matches the filter.
        {:else}
          <div class="empty-title flex items-center gap-2">
            <span class="inline-block size-2 rounded-full bg-ok animate-pulse"></span> Listening
          </div>
          Topics appear here as messages arrive. Press <kbd>/</kbd> to filter them.
        {/if}
      </div>
    {:else}
      {#each tree.children as k (k.path)}{@render row(k, 0)}{/each}
      <div class="h-1"></div>
    {/if}
  </div>

  {#if profileId}
    <div class="border-t border-line px-3 py-1.5 text-[11px] text-muted flex flex-wrap items-center gap-1.5">
      <span class="mr-0.5" title="Topic filters this connection is subscribed to">subscribed to</span>
      {#each subs as s (s.filter)}
        <span class="chip inline-flex items-center gap-1 font-mono text-ink pr-1" title="QoS {s.qos}">
          {s.filter}
          <button class="text-muted/70 hover:text-bad leading-none px-0.5" title="Unsubscribe from {s.filter}" onclick={() => app.unsubscribe(s.filter)}>×</button>
        </span>
      {/each}
      <form class="inline-flex" onsubmit={(e) => { e.preventDefault(); addSub(); }}>
        <input class="bg-transparent outline-none font-mono w-28 text-ink placeholder:text-muted/70" placeholder="+ filter"
               title="Subscribe to another filter, e.g. sensors/+/temp" bind:value={newFilter} spellcheck="false" />
      </form>
    </div>
  {/if}
</div>
