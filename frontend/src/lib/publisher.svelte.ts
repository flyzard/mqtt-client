// The publish draft. It lives outside the publish bar so the inspector can
// hand it a message ("edit in publisher") without the two components knowing
// about each other, and so the draft survives collapsing the bar.

import { layout } from "./prefs.svelte";
import type { QoS } from "./types";

export class Publisher {
  topic = $state("");
  payload = $state("");
  qos = $state<QoS>(0);
  retain = $state(false);
  repeat = $state(false);
  /** Seconds between repeated sends. */
  every = $state(5);
  /** Bumped whenever something asks the bar to put the caret in the payload. */
  focusRequest = $state(0);

  /** Replaces the draft with a message's topic and payload and opens the bar.
   *  Binary payloads cannot be edited as text and are left empty. */
  load(d: { topic: string; payload: string; qos?: QoS; retain?: boolean }) {
    this.topic = d.topic;
    this.payload = d.payload;
    if (d.qos !== undefined) this.qos = d.qos;
    if (d.retain !== undefined) this.retain = d.retain;
    layout.publishOpen.value = true;
    this.focusRequest++;
  }
}

export const publisher = new Publisher();
