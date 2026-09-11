<script lang="ts">
  // One message in full: metadata line, then the payload pretty-printed and
  // tinted. JSON lines whose value differs from the previous message on the
  // topic are washed in `warn`, so a stream of similar payloads reads as
  // "what moved" rather than a wall of text.
  import { fmtBytes, fmtTime } from "../lib/format";
  import { TOKEN_CLASS } from "../lib/highlight";
  import { jsonLines } from "../lib/jsonview";
  import { publisher } from "../lib/publisher.svelte";
  import { Transient } from "../lib/transient.svelte";
  import type { Msg } from "../lib/types";

  let {
    msg,
    previous,
    topic,
    pinned,
    onunpin,
  }: { msg: Msg; previous?: Msg; topic: string; pinned: boolean; onunpin: () => void } = $props();

  const json = $derived(msg.rendered.kind === "json");
  const lines = $derived(
    json ? jsonLines(msg.rendered.value, previous?.rendered.kind === "json" ? previous.rendered.value : undefined) : [],
  );
  const changed = $derived(lines.reduce((n, l) => n + (l.changed ? 1 : 0), 0));

  const copied = new Transient<"payload" | "topic">();
  function copy(what: "payload" | "topic") {
    void navigator.clipboard.writeText(what === "payload" ? msg.rendered.text : topic);
    copied.show(what);
  }

  function edit() {
    publisher.load({
      topic,
      payload: msg.rendered.kind === "hex" ? "" : msg.rendered.text,
      qos: msg.qos,
      retain: msg.retained,
    });
  }
</script>

<div class="h-full min-h-0 flex flex-col select-text cursor-auto">
  <div class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 pt-2 pb-1.5 text-[11px] text-muted">
    <span class="text-ink" title={pinned ? "Showing the message you picked" : "Following the newest message"}>
      {pinned ? "pinned" : "latest"}
    </span>
    <span class="font-mono tabular-nums">{fmtTime(msg.receivedAt)}</span>
    <span>{msg.rendered.kind}</span>
    <span>{fmtBytes(msg.bytes.length)}</span>
    <span>q{msg.qos}</span>
    {#if msg.retained}<span class="badge" title="retained">R</span>{/if}
    {#if msg.props.contentType}<span>{msg.props.contentType}</span>{/if}
    {#if msg.props.responseTopic}<span title="response topic">↩ {msg.props.responseTopic}</span>{/if}
    {#if msg.props.messageExpiry !== undefined}<span>expires in {msg.props.messageExpiry}s</span>{/if}
    {#each Object.entries(msg.props.user ?? {}) as [k, v] (k)}<span class="font-mono">{k}={v}</span>{/each}
    {#if changed}
      <span class="text-warn" title="Fields whose value differs from the previous message">{changed} changed</span>
    {/if}
    <span class="flex-1"></span>
    {#if pinned}
      <button class="btn-flat" title="Follow the newest message again (Esc)" onclick={onunpin}>follow latest</button>
    {/if}
    <button class="btn-flat" title="Copy the topic path" onclick={() => copy("topic")}>
      {copied.value === "topic" ? "copied" : "copy topic"}
    </button>
    {#if msg.rendered.kind !== "empty"}
      <button class="btn-flat" title="Copy the payload" onclick={() => copy("payload")}>
        {copied.value === "payload" ? "copied" : "copy payload"}
      </button>
    {/if}
    <button class="btn-flat" title="Load topic and payload into the publish bar" onclick={edit}>edit in publisher</button>
  </div>

  <div class="card flex-1 min-h-0 mx-4 mb-3 overflow-auto font-mono text-xs text-ink">
    {#if msg.rendered.kind === "empty"}
      <div class="px-3 py-2 text-muted/70 italic">empty payload</div>
    {:else if json}
      <div class="py-1.5">
        {#each lines as line, i (i)}
          <div class={["whitespace-pre-wrap break-all leading-5 pr-3", line.changed && "changed"]}
               style="padding-left: {12 + line.indent * 14}px"
          >{#each line.tokens as tok, j (j)}<span class={TOKEN_CLASS[tok.c]}>{tok.t}</span>{/each}</div>
        {/each}
      </div>
    {:else}
      <pre class="whitespace-pre-wrap break-all px-3 py-2">{msg.rendered.text}</pre>
    {/if}
  </div>
</div>
