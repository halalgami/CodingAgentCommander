<script>
  // Terminal font-size shortcuts: ⌘= / ⌘- / ⌘0. Window-level CAPTURE listener
  // (like ⌘K) so it works while xterm has focus. Matches ONLY these three
  // combos — ESC stays element-scoped per the focus-scoping rule.
  import { prefs, setPref } from "../prefs.svelte.js";
  import { toast } from "../stores.svelte.js";

  function onkey(e) {
    if (!e.metaKey || e.altKey || e.ctrlKey) return;
    let next = null;
    if (e.key === "=" || e.key === "+") next = Math.min(prefs.fontSize + 1, 20);
    else if (e.key === "-") next = Math.max(prefs.fontSize - 1, 11);
    else if (e.key === "0") next = 13;
    if (next === null || next === prefs.fontSize) { if (next !== null) e.preventDefault(); return; }
    e.preventDefault();
    e.stopPropagation();
    setPref("fontSize", next);
    toast(`Terminal font ${next}px`);
  }
  $effect(() => {
    window.addEventListener("keydown", onkey, true);
    return () => window.removeEventListener("keydown", onkey, true);
  });
</script>
