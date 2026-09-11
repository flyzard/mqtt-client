<script lang="ts">
  // Stats, sparkline and message list for one watched topic. Instantiated
  // under {#key topic}, so all state here lives exactly as long as the topic
  // is selected.
  import { Tween } from "svelte/motion";
  import { app } from "../lib/state.svelte";
  import { clock } from "../lib/clock.svelte";
  import { fmtBytes, fmtCount, fmtInterval, fmtRate, fmtTime, rateOf } from "../lib/format";
  import { numericFields, series } from "../lib/series";
  import Payload from "./Payload.svelte";
  import Sparkline from "./Sparkline.svelte";

  let { topic }: { topic: string } = $props();

  const stats = $derived(app.selectedProfile ? app.topics.get(app.selectedProfile)?.get(topic) : undefined);
  const latest = $derived(app.messages[0]);
  const rate = $derived(stats ? rateOf(stats, clock.now) : 0);
  // Eased so the tile ticks towards the new rate instead of jumping.
  const shownRate = new Tween(0, { duration: 400 });
  $effect(() => {
    shownRate.target = rate;
  });

  /** "0.48/s" -> ["0.48", "/s"] so the unit can be set small and dim. */
  function split(s: string): [string, string] {
    const m = /^([\d.,]+)(.*)$/.exec(s);
    return m ? [m[1], m[2]] : [s, ""];
  }

  // ---- sparkline ----------------------------------------------------------------
  const fields = $derived(numericFields(app.messages));
  let chosen = $state<string | null>(null);
  const field = $derived(chosen !== null && fields.includes(chosen) ? chosen : (fields[0] ?? null));
  const values = $derived(field === null ? [] : series(app.messages, field));

  // ---- list: expansion and "jump to latest" ---------------------------------------
  let expanded = $state<number | null>(null);
  let list = $state<HTMLDivElement>();
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

{#snippet tile(label: string, value: string)}
  {@const [num, unit] = split(value)}
  <div class="tile">
    <div class="text-[10px] uppercase tracking-wider text-muted">{label}</div>
    <div class="text-xl font-semibold tracking-tight text-ink tabular-nums leading-tight truncate">
      {num}<span class="text-xs font-normal tracking-normal text-muted ml-0.5">{unit}</span>
    </div>
  </div>
{/snippet}

<div class="h-full min-h-0 flex flex-col">
  <div class="grid grid-cols-4 gap-2 px-4 pt-3">
    {@render tile("msgs", fmtCount(stats?.count ?? 0))}
    {@render tile("rate", fmtRate(shownRate.current))}
    {@render tile("interval", fmtInterval(stats?.avgInterval ?? 0))}
    {@render tile("size", latest ? fmtBytes(latest.bytes.length) : "—")}
  </div>

  {#if field !== null}
    <div class="px-4 pt-3">
      <div class="flex items-center text-[11px] text-muted mb-1">
        <span>field</span>
        {#if fields.length > 1}
          <select class="ml-1.5 bg-transparent text-ink font-mono outline-none cursor-pointer" value={field}
                  onchange={(e) => (chosen = e.currentTarget.value)}>
            {#each fields as f (f)}<option value={f}>{f || "value"}</option>{/each}
          </select>
        {:else}
          <span class="ml-1.5 font-mono text-ink">{field || "value"}</span>
        {/if}
        <span class="flex-1"></span>
        <span>last {values.length}</span>
        <span class="ml-2 font-mono brand tabular-nums">{values.at(-1)?.v}</span>
      </div>
      <Sparkline points={values} />
    </div>
  {/if}

  <div class="flex items-center px-4 pt-3 pb-1 text-[11px] text-muted">
    <span>{app.messages.length} in view{app.dropped ? ` · ${app.dropped} skipped` : ""}</span>
    <span class="flex-1"></span>
    <button class="btn-flat" title="Forget this topic's history" onclick={() => app.clearTopic()}>clear</button>
  </div>

  <div class="relative flex-1 min-h-0 border-t border-line">
    {#if unseen > 0}
      <button class="btn-primary absolute right-4 top-2 z-10 rounded-full text-xs px-3 py-1" onclick={jump}>
        {unseen} new · jump to latest
      </button>
    {/if}
    <div bind:this={list} onscroll={onScroll} class="h-full overflow-y-auto">
      {#each app.messages as m (m.id)}
        {@const open = expanded === m.id}
        <div class="rise border-b border-line {open ? 'sel' : ''}">
          <button class="w-full text-left flex items-center gap-3 px-4 h-8 hover:bg-ink/3" onclick={() => (expanded = open ? null : m.id)}>
            <span class="font-mono text-xs text-muted tabular-nums shrink-0">{fmtTime(m.receivedAt)}</span>
            <span class="flex-1 min-w-0 truncate font-mono text-xs text-ink">
              {#if m.rendered.kind === "empty"}<span class="text-muted/70 italic">empty</span>{:else}<Payload value={m.rendered} max={200} />{/if}
            </span>
            {#if m.retained}<span class="badge" title="retained">R</span>{/if}
            <span class="text-[11px] text-muted shrink-0">q{m.qos}</span>
          </button>
          {#if open}
            <div class="px-4 pb-3 select-text cursor-auto">
              <div class="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted mb-1.5">
                <span>{m.rendered.kind}</span>
                <span>{fmtBytes(m.bytes.length)}</span>
                {#if m.props.contentType}<span>{m.props.contentType}</span>{/if}
                {#if m.props.responseTopic}<span title="response topic">↩ {m.props.responseTopic}</span>{/if}
                {#if m.props.messageExpiry !== undefined}<span>expires in {m.props.messageExpiry}s</span>{/if}
                {#each Object.entries(m.props.user ?? {}) as [k, v] (k)}<span class="font-mono">{k}={v}</span>{/each}
                <span class="flex-1"></span>
                <button class="btn-flat" onclick={() => navigator.clipboard.writeText(m.rendered.text)}>copy</button>
              </div>
              <pre class="card font-mono text-xs whitespace-pre-wrap break-all text-ink max-h-72 overflow-auto px-3 py-2">{m.rendered.text}</pre>
            </div>
          {/if}
        </div>
      {/each}
      {#if !app.messages.length}
        <div class="px-4 py-8 text-center text-xs text-muted">No messages yet</div>
      {/if}
    </div>
  </div>
</div>
