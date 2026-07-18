<script>
  import { onMount, onDestroy } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import "@xterm/xterm/css/xterm.css";
  import { WSPort, WSToken } from "../../wailsjs/go/main/App.js";
  import { prefs } from "./prefs.svelte.js";

  let { sessionKey = "", theme = null } = $props();
  let el, term, fit, ws, ro;
  let debounce;

  // Claude Code centers its content in wide terminals; the column cap keeps it
  // readable. Cap is a pref (0 = unlimited).
  function fitClamped() {
    if (!fit || !term) return;
    fit.fit();
    if (prefs.maxCols > 0 && term.cols > prefs.maxCols) term.resize(prefs.maxCols, term.rows);
  }
  function sendSize() {
    if (ws && ws.readyState === 1) {
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
    ws.onopen = () => refit();
    ws.onmessage = (e) => {
      if (typeof e.data === "string") return; // control/error text reserved
      term.write(new Uint8Array(e.data));
    };
  }

  onMount(() => {
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
