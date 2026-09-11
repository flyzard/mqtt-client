<script lang="ts">
  // Disclosure chevron. Points `dir` when closed and rotates a quarter turn
  // clockwise when `open`, so right becomes down. Drawn in currentColor so
  // the parent sets the shade; sized for a 16px hit box.
  let { open = false, dir = "right", size = 14 }: { open?: boolean; dir?: "right" | "left" | "down" | "up"; size?: number } = $props();
  const BASE = { right: 0, down: 90, left: 180, up: 270 };
  const angle = $derived(BASE[dir] + (open ? 90 : 0));
</script>

<svg width={size} height={size} viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.75"
     stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"
     class="shrink-0 transition-transform duration-150" style="transform: rotate({angle}deg)">
  <path d="M6 3.5 L10.5 8 L6 12.5" />
</svg>
