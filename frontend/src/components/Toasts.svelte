<script lang="ts">
  import { app } from "../lib/state.svelte";
</script>

{#if app.toasts.length}
  <div class="fixed bottom-3 right-3 flex flex-col gap-2 z-50 max-w-sm">
    {#each app.toasts as t (t.id)}
      <div class="card border-bad/40 text-ink text-xs px-3 py-2 flex gap-3 items-start shadow-soft" role="alert">
        <span class="size-1.5 mt-1.5 rounded-full bg-bad shrink-0"></span>
        <span class="flex-1 break-words">{t.text}</span>
        <button class="text-muted hover:text-ink" title="Dismiss" onclick={() => app.dismiss(t.id)}>×</button>
      </div>
    {/each}
  </div>
{/if}
