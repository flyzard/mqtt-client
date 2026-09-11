<script lang="ts">
  // One-line, syntax-tinted payload preview. The parent decides font and truncation.
  import { highlight, TOKEN_CLASS } from "../lib/highlight";
  import type { Rendered } from "../lib/payload";

  let { value, max = 120 }: { value: Rendered | null; max?: number } = $props();
  const tokens = $derived(highlight(value, max));
</script>

{#each tokens as tok, i (i)}<span class={TOKEN_CLASS[tok.c]}>{tok.t}</span>{/each}
