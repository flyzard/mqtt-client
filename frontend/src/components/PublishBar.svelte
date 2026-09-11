<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { layout } from "../lib/prefs.svelte";
  import { publisher as draft } from "../lib/publisher.svelte";
  import { Transient } from "../lib/transient.svelte";
  import { QOS_LEVELS } from "../lib/types";
  import { hostOf } from "../lib/broker";

  let payloadEl = $state<HTMLTextAreaElement>();
  const open = $derived(layout.publishOpen.value);

  // Prefill the topic whenever one is picked in the tree.
  $effect(() => {
    if (app.selectedTopic) draft.topic = app.selectedTopic;
  });
  // "Edit in publisher" from the inspector: the caret goes to the payload.
  $effect(() => {
    if (draft.focusRequest) void tick().then(() => payloadEl?.focus());
  });

  const profile = $derived(app.profiles.find((p) => p.id === app.selectedProfile));
  const connected = $derived(!!app.selectedProfile && app.stateOf(app.selectedProfile) === "connected");
  const canSend = $derived(connected && draft.topic.trim() !== "");

  // Cheap JSON hint, only for payloads that look like JSON: the parsed value,
  // "invalid", or null when it is not JSON-shaped at all.
  const json = $derived.by((): { value: unknown } | "invalid" | null => {
    const t = draft.payload.trim();
    if (!t || !/^[[{]/.test(t)) return null;
    try {
      return { value: JSON.parse(t) };
    } catch {
      return "invalid";
    }
  });

  function format() {
    if (json && json !== "invalid") draft.payload = JSON.stringify(json.value, null, 2);
  }

  // Brief confirmation after a successful publish; the message only shows up
  // in the list if the watched topic happens to match.
  const sent = new Transient<true>();
  async function send() {
    if (!canSend) return;
    const ok = await app.publish({ topic: draft.topic.trim(), text: draft.payload, qos: draft.qos, retain: draft.retain });
    if (ok) sent.show(true);
  }

  // Repeat while enabled and connected; re-armed when the interval changes.
  $effect(() => {
    if (!draft.repeat || !connected) return;
    const id = setInterval(send, Math.max(0.2, draft.every || 0) * 1000);
    return () => clearInterval(id);
  });

  function key(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      send();
    }
  }
</script>

<div class="shrink-0 border-t border-line px-4 py-2 flex flex-col gap-2">
  <div class="flex items-center gap-2 min-h-8">
    <button class="btn-flat -ml-2 px-1.5 text-[10px] text-muted shrink-0" aria-expanded={open}
            title={open ? "Collapse the publish bar" : "Expand the publish bar"}
            onclick={() => (layout.publishOpen.value = !open)}>
      {open ? "▾" : "▸"}
    </button>
    {#if open}
      <input class="inp accent {profile ? 'accent-on' : ''} flex-1 min-w-0 font-mono text-sm px-3 py-2" placeholder="topic"
             bind:value={draft.topic} onkeydown={key} spellcheck="false" autocomplete="off" />
    {:else}
      <button class="flex-1 min-w-0 flex items-center gap-3 text-left h-8" onclick={() => (layout.publishOpen.value = true)}>
        <span class="eyebrow">Publish</span>
        <span class="font-mono text-xs truncate {draft.topic ? 'text-ink' : 'text-muted/70'}">{draft.topic || "no topic yet"}</span>
        {#if draft.repeat && connected}
          <span class="text-ok text-[11px] animate-pulse shrink-0">● sending every {draft.every}s</span>
        {/if}
      </button>
    {/if}
    {#if profile}
      <span class="chip shrink-0 py-1 brand border-current/45" title={profile.broker}>
        {profile.name || hostOf(profile.broker)}
      </span>
    {/if}
  </div>

  {#if open}
    <textarea bind:this={payloadEl} class="inp w-full font-mono text-sm h-20 resize-y" placeholder="payload"
              bind:value={draft.payload} onkeydown={key} spellcheck="false"></textarea>

    <div class="flex items-center gap-3 text-xs text-muted">
      <label class="flex items-center gap-1">
        QoS
        <select bind:value={draft.qos}>
          {#each QOS_LEVELS as q (q)}<option value={q}>{q}</option>{/each}
        </select>
      </label>
      <label class="flex items-center gap-1 cursor-pointer"><input type="checkbox" bind:checked={draft.retain} /> retain</label>
      <label class="flex items-center gap-1 cursor-pointer">
        <input type="checkbox" bind:checked={draft.repeat} /> every
        <input type="number" min="0.2" step="0.5" class="inp w-12 px-1 py-0.5 text-ink tabular-nums" bind:value={draft.every} />
        s
      </label>
      {#if draft.repeat && connected}<span class="text-ok animate-pulse">● sending</span>{/if}
      {#if json === "invalid"}
        <span class="text-warn">invalid JSON</span>
      {:else if json}
        <button class="btn-flat -mx-2" title="Pretty-print the payload" onclick={format}>format json</button>
      {/if}
      <span class="flex-1"></span>
      {#if sent.value}<span class="text-ok">sent</span>{/if}
      <kbd title="Send">⌘↵</kbd>
      <button class="btn-primary" disabled={!canSend} title={connected ? "" : "Not connected"} onclick={send}>
        Send
      </button>
    </div>
  {/if}
</div>
