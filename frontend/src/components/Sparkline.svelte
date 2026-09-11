<script lang="ts">
  // Line chart of a numeric series: gradient fill under the line, a dot on
  // the newest point, min/max at the edges and a hover crosshair. Drawn in
  // the brand colour. The SVG is stretched to the container width, so
  // anything that must keep its shape (dot, crosshair, labels) is HTML placed
  // by percentage.
  import type { Point } from "../lib/series";
  import { fmtNum, fmtTime } from "../lib/format";

  let { points }: { points: Point[] } = $props();

  const W = 100;
  const H = 72;
  const PAD = 4;
  const uid = $props.id();

  const geo = $derived.by(() => {
    if (points.length < 2) return null;
    let min = Infinity;
    let max = -Infinity;
    for (const { v } of points) {
      if (v < min) min = v;
      if (v > max) max = v;
    }
    const span = max - min || 1;
    const xs: number[] = [];
    const ys: number[] = [];
    const parts: string[] = [];
    points.forEach(({ v }, i) => {
      const x = (i / (points.length - 1)) * W;
      const y = PAD + (1 - (v - min) / span) * (H - PAD * 2);
      xs.push(x);
      ys.push(y);
      parts.push(`${i ? "L" : "M"}${x.toFixed(2)},${y.toFixed(2)}`);
    });
    const line = parts.join(" ");
    return { min, max, xs, ys, line, area: `${line} L${W},${H} L0,${H} Z` };
  });

  // Index under the pointer, or null when not hovering.
  let hover = $state<number | null>(null);
  function move(e: PointerEvent) {
    const r = e.currentTarget as HTMLElement;
    const f = (e.clientX - r.getBoundingClientRect().left) / r.clientWidth;
    hover = Math.max(0, Math.min(points.length - 1, Math.round(f * (points.length - 1))));
  }
</script>

{#if geo}
  {@const last = points.length - 1}
  <div class="relative brand font-mono cursor-crosshair" style="height:{H}px" role="img" aria-label="chart"
       onpointermove={move} onpointerleave={() => (hover = null)}>
    <svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" class="absolute inset-0 block size-full" aria-hidden="true">
      <defs>
        <linearGradient id="g{uid}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="currentColor" stop-opacity="0.16" />
          <stop offset="1" stop-color="currentColor" stop-opacity="0" />
        </linearGradient>
      </defs>
      <path d={geo.area} fill="url(#g{uid})" />
      <path d={geo.line} fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
    </svg>

    <!-- Newest point -->
    <span class="absolute size-2 -m-1 rounded-full bg-current ring-2 ring-cream" style="left:{geo.xs[last]}%; top:{geo.ys[last]}px"></span>

    <!-- Range labels -->
    <span class="absolute left-0 top-0 text-[10px] leading-none text-muted tabular-nums">{fmtNum(geo.max)}</span>
    <span class="absolute left-0 bottom-0 text-[10px] leading-none text-muted tabular-nums">{fmtNum(geo.min)}</span>

    {#if hover !== null}
      <div class="absolute top-0 bottom-0 w-px bg-current opacity-40" style="left:{geo.xs[hover]}%"></div>
      <span class="absolute size-2 -m-1 rounded-full border border-current bg-cream" style="left:{geo.xs[hover]}%; top:{geo.ys[hover]}px"></span>
      <div class="card absolute top-0 text-[10px] leading-none whitespace-nowrap px-1.5 py-1 {geo.xs[hover] > 60 ? '-translate-x-full' : ''}"
           style="left:{geo.xs[hover]}%; margin-left:{geo.xs[hover] > 60 ? -6 : 6}px">
        <span class="text-ink tabular-nums">{fmtNum(points[hover].v)}</span>
        <span class="text-muted ml-1.5">{fmtTime(points[hover].t)}</span>
      </div>
    {/if}
  </div>
{:else}
  <div class="grid place-items-center text-[11px] text-muted/70" style="height:{H}px">waiting for a second value</div>
{/if}
