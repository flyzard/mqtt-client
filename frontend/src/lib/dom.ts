import { tick } from "svelte";

/** After the pending render, scrolls the row marked `[attr="value"]` inside
 *  `container` into view without jumping the list around. */
export async function reveal(container: Element | undefined, attr: string, value: string | number) {
  await tick();
  container?.querySelector(`[${attr}="${CSS.escape(String(value))}"]`)?.scrollIntoView({ block: "nearest" });
}
