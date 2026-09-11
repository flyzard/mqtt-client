<script lang="ts">
  // Stats, sparkline, message list and the detail pane for one watched
  // topic. Instantiated under {#key topic}, so all state here lives exactly
  // as long as the topic is selected.
  import { Tween } from "svelte/motion";
  import { app } from "../lib/state.svelte";
  import { atTick } from "../lib/clock.svelte";
  import { reveal } from "../lib/dom";
  import { bytesParts, fmtCount, fmtNum, fmtTime, intervalParts, rateOf, rateParts, type Quantity } from "../lib/format";
  import { layout } from "../lib/prefs.svelte";
  import { numericFields, series } from "../lib/series";
  import MessageDetail from "./MessageDetail.svelte";
  import Payload from "./Payload.svelte";
  import Resizer from "./Resizer.svelte";
  import Sparkline from "./Sparkline.svelte";

  let { topic }: { topic: string } = $props();

  const stats = $derived(app.selectedProfile ? app.topics.get(app.selectedProfile)?.get(topic) : undefined);
  const latest = $derived(app.messages[0]);
  // Sampled once per clock tick rather than per batch, and eased so the tile
  // ticks towards the new rate instead of jumping.
  const rate = $derived(atTick((now) => (stats ? rateOf(stats, now) : 0)));
  const shownRate = Tween.of(() => rate, { duration: 400 });
  // Rows that arrive after the view opened animate in; history does not.
  const openedAt = Date.now();

  // ---- sparkline ----------------------------------------------------------------
  const fields = $derived(numericFields(app.messages));
  let chosen = $state<string | null>(null);
  const field = $derived(chosen !== null && fields.includes(chosen) ? chosen : (fields[0] ?? null));
  const values = $derived(field === null ? [] : series(app.messages, field));

  // ---- detail: follow the newest message unless one is pinned ---------------------
  let pinned = $state<number | null>(null);
  const shownIndex = $derived.by(() => {
    if (pinned === null) return 0;
    const i = app.messages.findIndex((m) => m.id === pinned);
    return i < 0 ? 0 : i; // the pinned message was evicted: back to following
  });
  const shown = $derived(app.messages[shownIndex]);
  // Rows compare against the id, not the index: a prepend shifts every index
  // but only the two rows whose identity flips need to re-render.
  const shownId = $derived(shown?.id);
  const previous = $derived(app.messages[shownIndex + 1]);
  const isPinned = $derived(pinned !== null && shownId === pinned);

  let list = $state<HTMLDivElement>();
  function pin(id: number | null) {
    pinned = id;
    if (id !== null) void reveal(list, "data-id", id);
  }
  // Arrow keys walk the list while it has focus; Esc goes back to following.
  function listKeys(e: KeyboardEvent) {
    if (!app.messages.length) return;
    if (e.key === "ArrowDown") pin(app.messages[Math.min(app.messages.length - 1, shownIndex + 1)].id);
    else if (e.key === "ArrowUp") pin(shownIndex <= 1 ? null : app.messages[shownIndex - 1].id);
    else if (e.key === "Escape") pin(null);
    else return;
    e.preventDefault();
  }

  // ---- list: "jump to latest" -----------------------------------------------------
  // Newest id when the user scrolled away from the top; 0 while following.
  let anchor = $state(0);
  const unseen = $derived.by(() => {
    if (!anchor) return 0;
    const i = app.messages.findIndex((m) => m.id === anchor);
    return i < 0 ? app.messages.length : i;
  });
  function onScroll() {
    if ((list?.scrollTop ?? 0) <= 8) anchor = 0;
    else anchor ||= app.messages[0]?.id ?? 0;
  }
  function jump() {
    list?.scrollTo({ top: 0, behavior: "smooth" });
    anchor = 0;
  }
</script>

{#snippet tile(label: string, [num, unit]: Quantity)}
  <div class="card bg-cream px-3 py-2 min-w-0">
    <div class="eyebrow text-[10px]">{label}</div>
    <div class="text-xl font-semibold tracking-tight text-ink tabular-nums leading-tight truncate">
      {num}<span class="text-xs font-normal tracking-normal text-muted ml-0.5">{unit}</span>
    </div>
  </div>
{/snippet}

<div class="h-full min-h-0 flex flex-col">
  <div class="grid grid-cols-4 gap-2 px-4 pt-3">
    {@render tile("msgs", [fmtCount(stats?.count ?? 0)])}
    {@render tile("rate", rateParts(shownRate.current))}
    {@render tile("interval", intervalParts(stats?.avgInterval ?? 0))}
    {@render tile("size", bytesParts(latest?.bytes.length))}
  </div>

  {#if field !== null}
    <div class="px-4 pt-3">
      <div class="flex items-center text-[11px] text-muted mb-1">
        <span>field</span>
        {#if fields.length > 1}
          <select class="ml-1.5 font-mono" value={field}
                  onchange={(e) => (chosen = e.currentTarget.value)}>
            {#each fields as f (f)}<option value={f}>{f || "value"}</option>{/each}
          </select>
        {:else}
          <span class="ml-1.5 font-mono text-ink">{field || "value"}</span>
        {/if}
        <span class="flex-1"></span>
        <span>last {values.length}</span>
        <span class="ml-2 font-mono brand tabular-nums">{fmtNum(values[values.length - 1].v)}</span>
      </div>
      <Sparkline points={values} />
    </div>
  {/if}

  <div class="flex items-center px-4 pt-3 pb-1 text-[11px] text-muted">
    <span>{app.messages.length} in view{app.dropped ? ` · ${app.dropped} skipped` : ""}</span>
    {#if app.messages.length > 1}
      <span class="ml-3 text-muted/70">click a row to pin it · <kbd>↑</kbd><kbd>↓</kbd> to walk</span>
    {/if}
    <span class="flex-1"></span>
    <button class="btn-flat" title="Forget this topic's history" onclick={() => app.clearTopic()}>clear</button>
  </div>

  <div class="pane-list relative border-t border-line">
    {#if unseen > 0}
      <button class="btn-primary absolute right-4 top-2 z-10 rounded-full text-xs px-3 py-1" onclick={jump}>
        {unseen} new · jump to latest
      </button>
    {/if}
    <div bind:this={list} onscroll={onScroll} onkeydown={listKeys} tabindex="0" role="listbox"
         aria-label="messages" class="h-full overflow-y-auto outline-none">
      {#each app.messages as m (m.id)}
        {@const current = m.id === shownId}
        {@const unpins = current && isPinned}
        <div class="border-b border-line" class:sel={current} class:rise={m.receivedAt > openedAt} data-id={m.id}
             role="option" aria-selected={current}>
          <button class="w-full text-left flex items-center gap-3 px-4 h-8 hover:bg-ink/3"
                  title={unpins ? "Unpin and follow the newest message" : "Pin this message in the detail pane"}
                  onclick={() => pin(unpins ? null : m.id)}>
            <span class="font-mono text-xs text-muted tabular-nums shrink-0">{fmtTime(m.receivedAt)}</span>
            <span class="flex-1 min-w-0 truncate font-mono text-xs text-ink">
              {#if m.rendered.kind === "empty"}<span class="text-muted/70 italic">empty</span>{:else}<Payload value={m.rendered} max={200} />{/if}
            </span>
            {#if m.retained}<span class="badge" title="retained">R</span>{/if}
            <span class="text-[11px] text-muted shrink-0">q{m.qos}</span>
          </button>
        </div>
      {/each}
      {#if !app.messages.length}
        <div class="empty text-center">No messages yet</div>
      {/if}
    </div>
  </div>

  {#if shown}
    <div class="pane-detail relative border-t border-line"
         style="--detail: {layout.detail.value}px; --detail-min: {layout.detail.min}px">
      <Resizer axis="y" edge="start" bind:value={layout.detail.value}
               min={layout.detail.min} max={layout.detail.max} reset={layout.detail.initial} />
      <MessageDetail msg={shown} {previous} {topic} pinned={isPinned} onunpin={() => pin(null)} />
    </div>
  {/if}
</div>
