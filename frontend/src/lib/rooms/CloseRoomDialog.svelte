<script>
  // Close / leave / remove confirmations (UX spec §7). Each names what
  // happens; closing warns about anything not yet delivered.
  import { rooms, act, closePanel } from "./rooms.svelte.js";
  let { mode, sum, peers, onclose } = $props();
  let el = $state(null);
  $effect(() => { el?.focus(); });

  const waitingFor = $derived((peers || []).filter((p) => p.waiting > 0).map((p) => p.name));
  const q = $derived(sum.queued || 0);
  const TEXT = $derived({
    close: { title: `Close ${sum.name}?`, body: `Closes ${sum.name} for everyone. Messages stay readable on each machine; nobody can post. Sharing the project again starts a new room.`, go: "Close room", fn: "CloseRoom" },
    leave: { title: `Leave ${sum.name}?`, body: `You stop receiving ${sum.name} and can't post to it. Your copy stays readable here.`, go: "Leave room", fn: "LeaveRoom" },
    remove: { title: `Remove ${sum.name} from this machine?`, body: `Deletes this machine's copy. Other machines keep theirs, and it won't sync back here unless you restore it from Rooms → Hidden rooms.`, go: "Remove", fn: "RemoveRoomLocal" },
  }[mode]);

  async function go() {
    const id = sum.id;
    await act(TEXT.fn, id);
    if (!rooms.error) {
      onclose();
      if (mode === "remove") { rooms.current = ""; closePanel(); }
    }
  }
</script>

<div class="scrim">
  <div class="dlg" role="alertdialog" aria-modal="true" aria-labelledby="crd-t" tabindex="-1" bind:this={el} data-testid="room-close-dialog"
    onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); onclose(); } }}>
    <h4 id="crd-t">{TEXT.title}</h4>
    <p>{TEXT.body}</p>
    {#if mode === "close" && q}
      <p class="amber">{q === 1 ? "1 message hasn't" : `${q} messages haven't`} reached {waitingFor.join(", ") || "every machine"} — {q === 1 ? "it" : "they"}'ll stay unread there.</p>
    {/if}
    <div class="row">
      <button class="danger" data-testid="room-close-confirm" disabled={rooms.busy} onclick={go}>{TEXT.go}</button>
      <button onclick={onclose}>Cancel</button>
    </div>
  </div>
</div>

<style>
  .scrim { position: absolute; inset: 0; z-index: 40; background: rgba(0, 0, 0, 0.45); display: flex; align-items: center; justify-content: center; padding: var(--sp-2); }
  .dlg { background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: var(--sp-3, 12px); max-width: 320px; display: flex; flex-direction: column; gap: var(--sp-2); }
  .dlg:focus { outline: none; }
  h4, p { margin: 0; }
  .amber { color: #c98500; }
  .row { display: flex; gap: var(--sp-1); }
  .danger { background: var(--crit, #d03b3b); color: #fff; border: 0; border-radius: var(--r-1); padding: 4px 10px; cursor: pointer; }
</style>
