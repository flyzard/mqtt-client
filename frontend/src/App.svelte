<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { app } from "./lib/state.svelte";
  import { profileColor } from "./lib/color";
  import Sidebar from "./components/Sidebar.svelte";
  import TopicTree from "./components/TopicTree.svelte";
  import Inspector from "./components/Inspector.svelte";
  import PublishBar from "./components/PublishBar.svelte";
  import Toasts from "./components/Toasts.svelte";

  onMount(() => app.init());
  onDestroy(() => app.destroy());

  // The selected connection's identity colour: selection bars, chart line,
  // connection tag. Everything else stays charcoal.
  const brand = $derived(app.selectedProfile ? `--brand: ${profileColor(app.selectedProfile)}` : "");
  const crumbs = $derived(app.selectedTopic?.split("/") ?? []);
</script>

<div class="h-screen flex flex-col text-sm select-none" style={brand}>
  <!-- Title strip: draggable, and tall enough to clear the hidden-inset traffic lights on macOS.
       A barely-there wash of the connection colour, in the spirit of the hero gradient. -->
  <div class="h-10 shrink-0 grid grid-cols-[260px_340px_minmax(0,1fr)] items-end border-b border-line"
       style="--wails-draggable:drag; background: linear-gradient(to right, color-mix(in oklab, var(--brand) 6%, transparent), transparent 50%)">
    <div class="pl-[84px] pb-2 text-[11px] uppercase tracking-wider text-muted">Connections</div>
    <div class="pl-3 pb-2 text-[11px] uppercase tracking-wider text-muted">Topics</div>
    <div class="pl-4 pr-4 pb-2 text-xs truncate font-mono text-muted" title={app.selectedTopic ?? ""}>
      {#each crumbs as part, i (i)}
        {#if i}<span class="text-ink/30">/</span>{/if}<span class={i === crumbs.length - 1 ? "text-ink" : ""}>{part}</span>
      {/each}
    </div>
  </div>

  <div class="flex-1 min-h-0 grid grid-cols-[260px_340px_minmax(0,1fr)]">
    <aside class="min-h-0 border-r border-line"><Sidebar /></aside>
    <section class="min-h-0 border-r border-line"><TopicTree /></section>
    <main class="min-h-0 flex flex-col">
      <div class="flex-1 min-h-0"><Inspector /></div>
      <PublishBar />
    </main>
  </div>
</div>
<Toasts />
