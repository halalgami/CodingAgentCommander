import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { openPane, noteOutput, noteInput, msSinceOutput, msSinceInput, forget, openPaneCount, reset, ATTACH_REPLAY_MS, ECHO_WINDOW_MS, RESIZE_REPAINT_MS } from "./termbus.js";

const NOW = 1_700_000_000_000;

beforeEach(() => reset());

test("a pane with no output reads as infinitely stale, never as fresh", () => {
  // The slot ladder treats msSinceOutput < LIVE_MS as "producing output now".
  // A default of 0 would make every unseen pane permanently live.
  assert.equal(msSinceOutput("w1", NOW), Infinity);
  assert.equal(msSinceInput(NOW), Infinity);
});

test("noteOutput and noteInput are read back as elapsed milliseconds", () => {
  noteOutput("w1", NOW);
  assert.equal(msSinceOutput("w1", NOW + 250), 250);
  noteInput(NOW + 100);
  assert.equal(msSinceInput(NOW + 400), 300);
});

test("a session key and its bare window id are the SAME pane", () => {
  // sessionKey is windowID + ":" + Date.now(); the pane is recreated with a new
  // one on every switch, but it is the same pty.
  noteOutput("w1:1700000000000", NOW);
  assert.equal(msSinceOutput("w1", NOW + 10), 10);
  noteOutput("w1", NOW + 20);
  assert.equal(msSinceOutput("w1:9999", NOW + 30), 10);
});

test("output timestamps never go backwards on an out-of-order call", () => {
  noteOutput("w1", NOW + 1000);
  noteOutput("w1", NOW);            // a stale frame must not un-age the pane
  assert.equal(msSinceOutput("w1", NOW + 1000), 0);
});

test("input is global, not per pane: typing anywhere resets the idle gap", () => {
  noteInput(NOW);
  assert.equal(msSinceInput(NOW + 5), 5);
  noteOutput("w2", NOW + 100);
  assert.equal(msSinceInput(NOW + 5), 5, "output must not count as input");
});

test("openPane/close is balanced, so recreating a pane 200 times leaks nothing", () => {
  // The pane component is recreated on every session switch. Without a close in
  // onDestroy this counter climbs by one per switch and never comes down.
  assert.equal(openPaneCount(), 0);
  for (let i = 0; i < 200; i++) {
    const close = openPane("w1:" + i);
    assert.equal(openPaneCount(), 1, `two panes open at remount ${i}`);
    close();
  }
  assert.equal(openPaneCount(), 0);
});

test("the leak assertion is real: skipping close leaves panes open", () => {
  // If this fails, the previous test proves nothing.
  for (let i = 0; i < 3; i++) openPane("w" + i);
  assert.equal(openPaneCount(), 3);
});

test("close is idempotent and a second close cannot go negative", () => {
  const close = openPane("w1");
  close();
  close();
  assert.equal(openPaneCount(), 0);
});

test("openPane ignores an empty key and returns a usable no-op close", () => {
  const close = openPane("");
  assert.equal(typeof close, "function");
  close();
  assert.equal(openPaneCount(), 0);
});

test("forget drops one pane's record without touching the others", () => {
  noteOutput("w1", NOW);
  noteOutput("w2", NOW);
  forget("w1:123");
  assert.equal(msSinceOutput("w1", NOW), Infinity);
  assert.equal(msSinceOutput("w2", NOW), 0);
});

test("reset clears everything, so tests cannot bleed into each other", () => {
  noteOutput("w1", NOW);
  noteInput(NOW);
  openPane("w1");
  reset();
  assert.equal(msSinceOutput("w1", NOW), Infinity);
  assert.equal(msSinceInput(NOW), Infinity);
  assert.equal(openPaneCount(), 0);
});

// Attaching to a pane replays what is already on it. Recording that as
// activity made every session look like it had just done something the moment
// it was selected — and once the "still working" window was widened, that lie
// lasted for as long as the window.
test("the attach-replay window is long enough to cover a replay burst", () => {
  // Not a behaviour assertion so much as a lock on the number: too small and
  // the replay leaks through, too large and genuinely live output is ignored
  // for that long after every switch.
  assert.ok(ATTACH_REPLAY_MS >= 250, "too short to cover a replay burst");
  assert.ok(ATTACH_REPLAY_MS <= 1500, "long enough to swallow real output");
});

// The suppression lives in Terminal.svelte, but the consequence is here: a
// timestamp only ever means "real output arrived", so a stale one reads as
// quiet rather than as busy.
test("a pane never seen produces Infinity, not a fresh-looking zero", () => {
  forget("never-seen");
  assert.equal(msSinceOutput("never-seen", 1_000), Infinity,
    "a small number would make an unvisited pane look live");
});

// Typing is OUTPUT — the pty echoes it back, measured at 27 bytes for 25
// keystrokes. Counted as activity it made merely typing report a pane as busy
// for the whole liveness window. The suppression lives in Terminal.svelte;
// what is asserted here is the contract it relies on.
test("the echo window is short enough not to swallow real output", () => {
  assert.ok(ECHO_WINDOW_MS >= 100, "too short to cover echo round-trip");
  assert.ok(ECHO_WINDOW_MS <= 500, "long enough to hide real output after a keystroke");
});

// noteInput must be observable from the same clock the suppression compares
// against, or the two drift and echo leaks through.
test("input recency is readable immediately after a keystroke", () => {
  noteInput(5_000);
  assert.equal(msSinceInput(5_000), 0);
  assert.equal(msSinceInput(5_100), 100);
});

// The THIRD source of output that is not work, after the attach replay and
// terminal echo. A resize sends SIGWINCH and a full-screen TUI redraws
// everything, so resizing the window or dragging a font slider made a session
// that finished an hour ago announce itself as busy.
test("the resize-repaint window covers a full redraw without hiding real work", () => {
  assert.ok(RESIZE_REPAINT_MS >= ECHO_WINDOW_MS,
    "a full repaint takes longer to arrive than a single character's echo");
  assert.ok(RESIZE_REPAINT_MS <= 1500, "long enough to hide genuine output");
});
