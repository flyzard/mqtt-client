<script lang="ts">
  import { app } from "../lib/state.svelte";
  import TopicView from "./TopicView.svelte";
</script>

{#if app.selectedTopic}
  <!-- Keyed so every piece of per-topic view state is recreated on switch. -->
  {#key app.selectedTopic}
    <TopicView topic={app.selectedTopic} />
  {/key}
{:else}
  <div class="h-full grid place-items-center">
    <div class="text-center text-muted text-xs leading-relaxed max-w-xs">
      <div class="text-ink font-semibold text-base tracking-tight mb-1">Pick a topic</div>
      Messages, rate and a live chart of any numeric field show up here.
      <div class="mt-3 text-muted/80">Press <kbd>/</kbd> to filter topics · <kbd>⌘↵</kbd> to publish</div>
    </div>
  </div>
{/if}
