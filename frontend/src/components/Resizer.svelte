<script lang="ts">
  // Drag handle on one edge of a pane. It sits over the pane's hairline
  // border (the parent must be `relative`), widens the hit area to 7px and
  // shows a faint wash on hover and while dragging. Double-click resets the
  // size to its default.
  import { clamp } from "../lib/format";

  let {
    value = $bindable(),
    min,
    max,
    reset,
    axis = "x",
    edge = "end",
  }: {
    value: number;
    min: number;
    max: number;
    reset: number;
    axis?: "x" | "y";
    /** Which edge of the pane the handle sits on. On the start edge a drag
     *  towards the positive axis shrinks the pane. */
    edge?: "start" | "end";
  } = $props();

  let dragging = $state(false);
  const sign = $derived(edge === "start" ? -1 : 1);

  function down(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    const el = e.currentTarget as HTMLElement;
    const start = axis === "x" ? e.clientX : e.clientY;
    const base = value;
    const ac = new AbortController();
    const opts = { signal: ac.signal };
    dragging = true;
    document.body.style.cursor = axis === "x" ? "col-resize" : "row-resize";
    el.setPointerCapture(e.pointerId);
    el.addEventListener("pointermove", (ev) => {
      const d = (axis === "x" ? ev.clientX : ev.clientY) - start;
      value = Math.round(clamp(base + sign * d, min, max));
    }, opts);
    const up = () => {
      dragging = false;
      document.body.style.cursor = "";
      ac.abort();
    };
    el.addEventListener("pointerup", up, opts);
    el.addEventListener("pointercancel", up, opts);
  }
</script>

<div
  role="separator"
  aria-orientation={axis === "x" ? "vertical" : "horizontal"}
  aria-valuenow={value}
  aria-valuemin={min}
  aria-valuemax={max}
  title="Drag to resize · double-click to reset"
  class={["resizer", `resizer-${axis}`, `resizer-${edge}`, dragging && "resizer-on"]}
  onpointerdown={down}
  ondblclick={() => (value = reset)}
></div>
