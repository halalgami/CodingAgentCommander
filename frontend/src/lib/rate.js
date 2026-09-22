// Rate limiting for continuous inputs.
//
// A range slider fires oninput on every step. Dragging one across its range
// emits hundreds of events in a couple of seconds, and several of this app's
// sliders land on a handler that writes a file (companionSetSize -> a temp
// write plus a rename, plus a native window resize) or serializes the whole of
// localStorage (setPref). The visual feedback has to stay per-event; the
// persistence does not.

// trailing returns a wrapper that runs fn at most once per `ms` of quiet, with
// the LAST arguments it was called with. Use it for writes that only need the
// final value: the intermediate states of a drag are not worth persisting.
export function trailing(fn, ms = 120) {
  let t = null;
  let last = null;
  const wrapped = (...args) => {
    last = args;
    if (t !== null) clearTimeout(t);
    t = setTimeout(() => {
      t = null;
      const a = last;
      last = null;
      fn(...a);
    }, ms);
  };
  // flush applies a pending call immediately — for pointerup, or teardown, where
  // waiting for the timer would drop the value the user actually chose.
  wrapped.flush = () => {
    if (t === null) return;
    clearTimeout(t);
    t = null;
    const a = last;
    last = null;
    fn(...a);
  };
  wrapped.cancel = () => {
    if (t !== null) clearTimeout(t);
    t = null;
    last = null;
  };
  return wrapped;
}
