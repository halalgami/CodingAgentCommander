<script>
  import { onMount, onDestroy } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import "@xterm/xterm/css/xterm.css";
  import { WSPort, WSToken } from "../../wailsjs/go/main/App.js";
  import { prefs } from "./prefs.svelte.js";
  import { openPane, noteOutput, noteInput, msSinceInput, ATTACH_REPLAY_MS, ECHO_WINDOW_MS, RESIZE_REPAINT_MS } from "./termbus.js";

  let { sessionKey = "", theme = null } = $props();
  let el, term, fit, ws, ro, closePane;
  let debounce;

  // Claude Code centers its content in wide terminals; the column cap keeps it
  // readable. Cap is a pref (0 = unlimited).
  function fitClamped() {
    if (!fit || !term) return;
    fit.fit();
    if (prefs.maxCols > 0 && term.cols > prefs.maxCols) term.resize(prefs.maxCols, term.rows);
  }
  // Stamped whenever a resize goes out, so the repaint it provokes can be told
  // apart from the session actually doing something. Module-scoped alongside
  // `ws` rather than local to connect(), because refit() fires from the
  // ResizeObserver and the prefs effect too, not only from the socket path.
  let resizedAt = 0;
  function sendSize() {
    if (ws && ws.readyState === 1) {
      resizedAt = Date.now();
      ws.send(JSON.stringify({ type: "resize", rows: term.rows, cols: term.cols }));
    }
  }
  function refit() {
    fitClamped();
    sendSize();
  }
  function refitSoon() {
    clearTimeout(debounce);
    debounce = setTimeout(refit, 50);
  }

  async function connect() {
    if (ws) { ws.close(); ws = null; }
    const port = await WSPort();
    const token = await WSToken();
    ws = new WebSocket(`ws://127.0.0.1:${port}/ws?token=${encodeURIComponent(token)}`);
    ws.binaryType = "arraybuffer";
    let attachedAt = 0;
    ws.onopen = () => { attachedAt = Date.now(); refit(); };
    ws.onmessage = (e) => {
      if (typeof e.data === "string") return; // control/error text reserved
      // The frames that arrive immediately after attaching are tmux replaying
      // what is ALREADY on the pane. That is history, not activity, and
      // recording it made every session look like it had just done something
      // the moment you selected it. The bytes are still written — only the
      // activity timestamp is withheld.
      // THREE things arrive here that are not the session doing work: the
      // burst tmux replays on attach, the pane echoing back what the user
      // typed, and the full repaint a resize triggers via SIGWINCH. All are
      // written to the terminal; none is activity.
      //
      // The pattern is worth naming, because each was found separately and the
      // same shape will recur: real bytes, false implication. Anything that
      // makes the pane redraw without the session having done anything belongs
      // in this list.
      const now = Date.now();
      const isReplay = now - attachedAt <= ATTACH_REPLAY_MS;
      const isEcho = msSinceInput(now) <= ECHO_WINDOW_MS;
      const isRepaint = now - resizedAt <= RESIZE_REPAINT_MS;
      if (!isReplay && !isEcho && !isRepaint) noteOutput(sessionKey, now);
      term.write(new Uint8Array(e.data));
    };
  }

  onMount(() => {
    closePane = openPane(sessionKey);
    term = new Terminal({
      fontSize: prefs.fontSize,
      scrollback: prefs.scrollback,
      cursorBlink: true,
      theme: theme ?? undefined,
    });
    fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);
    fitClamped();
    term.onData((d) => {
      noteInput();
      if (ws && ws.readyState === 1) ws.send(new TextEncoder().encode(d));
    });
    // ResizeObserver on the container catches window resizes, sidebar drags,
    // and UI-scale changes; the old window-resize listener missed layout-only
    // changes and is kept only as a fallback for ancient webviews.
    if (typeof ResizeObserver !== "undefined") {
      ro = new ResizeObserver(refitSoon);
      ro.observe(el);
    } else {
      window.addEventListener("resize", refitSoon);
    }
  });

  $effect(() => {
    if (term && sessionKey) connect();
  });

  // xterm theme is a whole-object live setter; spread reads every field so the
  // effect tracks nested $state mutations (live accent slider).
  $effect(() => {
    if (term && theme) term.options.theme = { ...theme };
  });

  // Live pref application — reading the fields registers them as deps.
  $effect(() => {
    if (!term) return;
    const { fontSize, scrollback, maxCols } = prefs;
    void maxCols; // tracked: cap changes must also trigger a refit
    term.options.fontSize = fontSize;
    term.options.scrollback = scrollback;
    refitSoon();
  });

  onDestroy(() => {
    closePane?.();
    closePane = null;
    if (ro) ro.disconnect();
    window.removeEventListener("resize", refitSoon);
    clearTimeout(debounce);
    if (ws) ws.close();
    if (term) term.dispose();
  });
</script>

<div class="term" bind:this={el}></div>
<style>
  /* Flex-center keeps a col-capped terminal from hugging the left edge with a
     dead void beside it; uncapped terminals fill the pane as before. */
  .term { width: 100%; height: 100%; text-align: left; display: flex; justify-content: center; }
</style>
