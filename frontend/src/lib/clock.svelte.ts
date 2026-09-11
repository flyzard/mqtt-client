// A single shared 1 Hz clock so every "3s ago" label in the app ticks
// together instead of each row owning a timer. It pauses while the window is
// hidden: nothing is painted, so nothing needs re-deriving.

export const clock = $state({ now: Date.now() });

setInterval(() => {
  if (!document.hidden) clock.now = Date.now();
}, 1000);
