<script lang="ts">
  import { app } from "../lib/state.svelte";
  import { QOS_LEVELS, type QoS } from "../lib/types";
  import { hostOf } from "../lib/broker";

  let topic = $state("");
  let payload = $state("");
  let qos = $state<QoS>(0);
  let retain = $state(false);
  let repeat = $state(false);
  let every = $state(5);

  // Prefill the topic whenever one is picked in the tree.
  $effect(() => {
    if (app.selectedTopic) topic = app.selectedTopic;
  });

  const profile = $derived(app.profiles.find((p) => p.id === app.selectedProfile));
  const connected = $derived(!!app.selectedProfile && app.stateOf(app.selectedProfile) === "connected");
  const canSend = $derived(connected && topic.trim() !== "");

  // Cheap JSON hint: only for payloads that look like JSON.
  const json = $derived.by(() => {
    const t = payload.trim();
    if (!t || !/^[[{]/.test(t)) return null;
    try {
      JSON.parse(t);
      return "valid";
    } catch {
      return "invalid";
    }
  });

  function send() {
    if (!canSend) return;
    void app.publish({ topic: topic.trim(), text: payload, qos, retain });
  }

  // Repeat while enabled and connected; re-armed when the interval changes.
  $effect(() => {
    if (!repeat || !connected) return;
    const id = setInterval(send, Math.max(0.2, every || 0) * 1000);
    return () => clearInterval(id);
  });

  function key(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      send();
    }
  }
</script>

<div class="border-t border-line px-4 py-3 flex flex-col gap-2">
  <div class="flex items-center gap-2">
    <input class="inp accent {profile ? 'accent-on' : ''} flex-1 min-w-0 font-mono text-sm px-3 py-2" placeholder="topic"
           bind:value={topic} onkeydown={key} spellcheck="false" autocomplete="off" />
    {#if profile}
      <span class="chip shrink-0 py-1 brand border-current/45" title={profile.broker}>
        {profile.name || hostOf(profile.broker)}
      </span>
    {/if}
  </div>

  <textarea class="inp w-full font-mono text-sm h-20 resize-y" placeholder="payload" bind:value={payload}
            onkeydown={key} spellcheck="false"></textarea>

  <div class="flex items-center gap-3 text-xs text-muted">
    <label class="flex items-center gap-1">
      QoS
      <select bind:value={qos}>
        {#each QOS_LEVELS as q (q)}<option value={q}>{q}</option>{/each}
      </select>
    </label>
    <label class="flex items-center gap-1 cursor-pointer"><input type="checkbox" bind:checked={retain} /> retain</label>
    <label class="flex items-center gap-1 cursor-pointer">
      <input type="checkbox" bind:checked={repeat} /> every
      <input type="number" min="0.2" step="0.5" class="inp w-12 px-1 py-0.5 text-ink tabular-nums" bind:value={every} />
      s
    </label>
    {#if repeat && connected}<span class="text-ok animate-pulse">● sending</span>{/if}
    {#if json === "invalid"}<span class="text-warn">invalid JSON</span>{:else if json === "valid"}<span>json</span>{/if}
    <span class="flex-1"></span>
    <kbd title="Send">⌘↵</kbd>
    <button class="btn-primary" disabled={!canSend} title={connected ? "" : "Not connected"} onclick={send}>
      Send
    </button>
  </div>
</div>
