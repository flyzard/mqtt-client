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
    <div class="empty text-center max-w-xs">
      <div class="empty-title text-base tracking-tight">Pick a topic</div>
      Messages, rate and a live chart of any numeric field show up here.
      <div class="mt-3 text-muted/80 leading-loose">
        <kbd>/</kbd> filter topics · <kbd>↑</kbd><kbd>↓</kbd> walk them · <kbd>⌘↵</kbd> publish
      </div>
    </div>
  </div>
{/if}
