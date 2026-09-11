<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { app } from "./lib/state.svelte";
  import { profileColor } from "./lib/color";
  import { clamp } from "./lib/format";
  import { layout } from "./lib/prefs.svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import TopicTree from "./components/TopicTree.svelte";
  import Inspector from "./components/Inspector.svelte";
  import PublishBar from "./components/PublishBar.svelte";
  import Resizer from "./components/Resizer.svelte";
  import Toasts from "./components/Toasts.svelte";

  onMount(() => app.init());
  onDestroy(() => app.destroy());

  // The selected connection's identity colour: selection bars, chart line,
  // connection tag. Everything else stays charcoal.
  const brand = $derived(app.selectedProfile ? `--brand: ${profileColor(app.selectedProfile)}` : "");
  const crumbs = $derived(app.selectedTopic?.split("/") ?? []);

  // Pane widths come from the persisted layout; the title strip shares the
  // template so its labels stay aligned. The inspector always keeps room.
  let innerWidth = $state(1280);
  const sidebarOpen = $derived(layout.sidebarOpen.value);
  const sidebarWidth = $derived(sidebarOpen ? layout.sidebar.value : layout.rail);
  const treeMax = $derived(clamp(innerWidth - sidebarWidth - layout.inspectorMin, layout.tree.min, layout.tree.max));
  const cols = $derived(`--grid-template-columns-panes: ${sidebarWidth}px ${layout.tree.value}px minmax(0, 1fr)`);
  // The macOS traffic lights sit in the first 84px of the title strip; when the
  // sidebar is folded to a rail the Topics label has to clear them instead.
  const topicsPad = $derived(sidebarOpen ? 12 : Math.max(12, 84 - layout.rail));
</script>

<svelte:window bind:innerWidth />

<div class="h-screen flex flex-col text-sm select-none" style="{brand}; {cols}">
  <!-- Title strip: draggable, and tall enough to clear the hidden-inset traffic lights on macOS.
       A barely-there wash of the connection colour, in the spirit of the hero gradient. -->
  <div class="h-10 shrink-0 grid grid-cols-panes items-end border-b border-line bg-linear-to-r from-(--brand)/6 to-transparent to-50%"
       style="--wails-draggable:drag">
    <div class="eyebrow pl-[84px] pb-2 truncate">{sidebarOpen ? "Connections" : ""}</div>
    <div class="eyebrow pb-2" style="padding-left: {topicsPad}px">Topics</div>
    <div class="pl-4 pr-4 pb-2 text-xs truncate font-mono text-muted" title={app.selectedTopic ?? ""}>
      {#each crumbs as part, i (i)}
        {#if i}<span class="text-ink/30">/</span>{/if}<span class={i === crumbs.length - 1 ? "text-ink" : ""}>{part}</span>
      {/each}
    </div>
  </div>

  <div class="flex-1 min-h-0 grid grid-cols-panes">
    <aside class="relative min-h-0 border-r border-line">
      <Sidebar />
      {#if sidebarOpen}
        <Resizer bind:value={layout.sidebar.value} min={layout.sidebar.min} max={layout.sidebar.max} reset={layout.sidebar.initial} />
      {/if}
    </aside>
    <section class="relative min-h-0 border-r border-line">
      <TopicTree />
      <Resizer bind:value={layout.tree.value} min={layout.tree.min} max={treeMax} reset={layout.tree.initial} />
    </section>
    <main class="min-h-0 flex flex-col">
      <div class="flex-1 min-h-0 overflow-hidden"><Inspector /></div>
      <PublishBar />
    </main>
  </div>
</div>
<Toasts />
