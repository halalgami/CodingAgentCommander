// Terminal activity bus.
//
// A tiny module-scope record of when each pane last produced output and when the
// user last typed into any pane. Panes publish; anything that needs to know
// whether a pane is live right now reads. Keeping it here means the websocket
// handler stays a single write instead of every interested module subscribing
// to the hottest code path in the app.
//
// DELIBERATELY NOT REACTIVE. This is a plain .js module, not .svelte.js.
// noteOutput runs once per websocket frame; a reactive write per frame would
// schedule a framework flush per frame and turn a 50k-line flood into thousands
// of re-renders. Readers poll on their own clock instead.
//
// Keys normalise to the window id. A pane's session key is
// `windowID + ":" + Date.now()` and the pane component is recreated with a fresh
// one on every switch, so keying on the raw value would add a permanent map
// entry per switch. Same pty, same key.

const outputAt = new Map();   // windowID -> ms of the last frame
const openPanes = new Map();  // windowID -> open count
let inputAt = -Infinity;

function baseID(key) {
  return String(key ?? "").split(":")[0];
}

// openPane records a live pane and returns its teardown. The caller MUST invoke
// the returned function when the pane goes away; openPaneCount() is what the
// test asserts on to prove it does.
export function openPane(key) {
  const id = baseID(key);
  if (!id) return () => {};
  openPanes.set(id, (openPanes.get(id) ?? 0) + 1);
  let closed = false;
  return function close() {
    if (closed) return;
    closed = true;
    const n = (openPanes.get(id) ?? 0) - 1;
    if (n > 0) openPanes.set(id, n);
    else openPanes.delete(id);
  };
}

// How long after a pane attaches its frames are treated as HISTORY rather than
// activity. tmux replays the pane's existing contents on attach, so without
// this a session that has been idle for hours reads as live the instant you
// click it — and every session you visit looks busy.
//
// Generic name and wording: this file survives the public export.
export const ATTACH_REPLAY_MS = 600;

// How long after a keystroke the pane's own echo is expected back.
//
// Typing is OUTPUT: the pty echoes every character, so 25 keystrokes produce 27
// bytes of output (measured). Counted as activity, that made merely typing
// report the pane as busy for the whole liveness window — which is what a user
// saw as "it says working and nothing is working".
//
// Echo is not distinguishable from real output by content, but it is by TIMING:
// it arrives within milliseconds of the keystroke that caused it. Suppressing a
// window after input costs nothing when something IS running, because the next
// frame lands outside it.
export const ECHO_WINDOW_MS = 250;

// How long after a pty RESIZE the repaint it triggers is expected back.
//
// A resize sends SIGWINCH and a full-screen TUI redraws everything, which is a
// large burst of output that has nothing to do with the session working.
// Resizing the window, dragging the font-size or width-cap sliders, or even
// toggling a panel all produce it — so a session that finished an hour ago
// announced itself as busy for the whole liveness window.
//
// Longer than the echo window because a full repaint of a large pane arrives in
// several frames.
export const RESIZE_REPAINT_MS = 700;

export function noteOutput(key, nowMs = Date.now()) {
  const id = baseID(key);
  if (!id) return;
  const prev = outputAt.get(id) ?? -Infinity;
  if (nowMs > prev) outputAt.set(id, nowMs);
}

export function noteInput(nowMs = Date.now()) {
  if (nowMs > inputAt) inputAt = nowMs;
}

// Infinity, not 0, for a pane that has never produced output: callers treat a
// small number as "live right now", so 0 would make every unseen pane live.
export function msSinceOutput(key, nowMs = Date.now()) {
  const at = outputAt.get(baseID(key));
  return at === undefined ? Infinity : nowMs - at;
}

export function msSinceInput(nowMs = Date.now()) {
  return inputAt === -Infinity ? Infinity : nowMs - inputAt;
}

export function forget(key) {
  const id = baseID(key);
  outputAt.delete(id);
  openPanes.delete(id);
}

export function openPaneCount() {
  let n = 0;
  for (const c of openPanes.values()) n += c;
  return n;
}

// Tests only. Nothing in the app calls this.
export function reset() {
  outputAt.clear();
  openPanes.clear();
  inputAt = -Infinity;
}
