// A value shown briefly and then cleared, for "copied" and "sent" confirmations.

export class Transient<T> {
  value = $state<T | null>(null);
  #timer: ReturnType<typeof setTimeout> | undefined;

  show(v: T, ms = 1200) {
    this.value = v;
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => (this.value = null), ms);
  }
}
