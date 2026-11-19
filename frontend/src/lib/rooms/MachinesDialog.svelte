<script>
  // Machines & network (UX spec §8, §11): my machines and their networks,
  // add a machine, join with a code, invite a colleague, reset.
  import { rooms, act, summaryOf } from "./rooms.svelte.js";
  import { netLabel } from "./rooms.js";
  import * as App from "../../../wailsjs/go/main/App.js";
  import NetworkCards from "./NetworkCards.svelte";

  let el = $state(null);
  let list = $state([]);
  let net = $state("");
  let codeIn = $state("");
  let word = $state("");
  let now = $state(Date.now());
  const mode = $derived(rooms.machines);
  const st = $derived(rooms.status);
  const join = $derived(st?.join);
  const room = $derived(summaryOf(rooms.current));

  $effect(() => { el?.focus(); });
  $effect(() => { const iv = setInterval(() => (now = Date.now()), 1000); return () => clearInterval(iv); });
  $effect(() => { if (mode === "list") App.Machines?.().then((m) => (list = m || [])).catch(() => {}); });
  $effect(() => { mode; net = ""; rooms.code = null; rooms.error = ""; });
  // A pasted CMDR code lands here (1.5b); the user still presses Join.
  $effect(() => { if (rooms.joinCode) { codeIn = rooms.joinCode; rooms.joinCode = ""; } });

  const left = (exp) => { const s = Math.max(0, exp - Math.floor(now / 1000)); return s >= 3600 ? `${Math.floor(s / 3600)} h` : `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`; };
  async function pick(n) {
    net = n;
    rooms.code = mode === "invite" ? await act("InviteCode", rooms.current, n) : await act("PairCode", n);
  }
  async function copy() { try { await navigator.clipboard.writeText(rooms.code.code); } catch {} }
  async function doJoin() {
    if (!codeIn.trim()) return;
    rooms.code = await act("JoinCode", codeIn.trim(), "");
  }
  function close() {
    if (rooms.code?.code) act("CancelCode");
    rooms.machines = null;
  }
  const TITLE = { list: "Machines & network", add: "Add a machine", join: "Join with a code", invite: "Invite a colleague", reset: "Reset rooms on this machine" };
</script>

<div class="scrim">
  <div class="dlg" role="dialog" aria-modal="true" aria-labelledby="md-t" tabindex="-1" bind:this={el} data-testid="machines-dialog"
    onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); close(); } }}>
    <header><h3 id="md-t">{TITLE[mode]}</h3><button class="icon" aria-label="Close" onclick={close}>✕</button></header>
    {#if rooms.error}<p class="err" role="alert">{rooms.error}</p>{/if}

    {#if mode === "list"}
      <p class="dim">This machine: <strong>{st?.machine}</strong>{st?.name ? ` · ${st.name}` : ""}</p>
      {#each list as m (m.device)}
        <div class="row" data-testid="machine-row">
          <span class="nm">{m.name}</span>
          <select aria-label={"Network for " + m.name} value={m.net} onchange={(e) => act("SetDeviceNetwork", m.device, e.currentTarget.value).then(() => App.Machines?.().then((x) => (list = x || [])))}>
            {#if !m.net}<option value="" disabled>Choose…</option>{/if}
            <option value="lan">{netLabel("lan")}</option>
            <option value="tailscale">{netLabel("tailscale")}</option>
          </select>
          <span class="dim small">{m.state}{m.reason ? " — " + m.reason : ""}</span>
        </div>
      {:else}
        <p class="dim">No other machines yet.</p>
      {/each}
      <div class="btns">
        {#if st?.first}<button class="primary" data-testid="machines-add" onclick={() => (rooms.machines = "add")}>Add a machine…</button>{/if}
        <button data-testid="machines-tojoin" onclick={() => (rooms.machines = "join")}>Join with a code…</button>
        <button class="danger-link" data-testid="machines-reset" onclick={() => (rooms.machines = "reset")}>Reset rooms on this machine…</button>
      </div>

    {:else if mode === "add" || mode === "invite"}
      {#if mode === "invite"}<p>Invite a colleague to <strong>{room?.name}</strong>. How will their machine connect to this one?</p>
      {:else}<p>How will the other machine connect to this one?</p>{/if}
      <NetworkCards value={net} onpick={pick} />
      {#if rooms.code?.code}
        <label class="code">Code ({netLabel(rooms.code.net)} · {left(rooms.code.expires)} left)
          <textarea readonly rows="3" data-testid="machines-code">{rooms.code.code}</textarea></label>
        <button class="primary" data-testid="machines-copy" onclick={copy}>Copy</button>
        {#if mode === "invite"}
          <p class="dim small">Send this to your colleague. It works once, for 24 hours. They'll see this room's messages.</p>
        {:else}
          <p class="dim small">On the other machine: Commander → Rooms → Join with a code… → paste. Then check the 4 words match on both.</p>
        {/if}
      {/if}

    {:else if mode === "join"}
      {#if !join || join.status !== "waiting"}
        {#if join?.status === "done"}<p class="dim">The last code worked. Paste another to join again.</p>{/if}
        <label>Paste the code <input placeholder="CMDR-…" data-testid="machines-code-in" bind:value={codeIn} /></label>
        {#if join?.status === "failed" && join.error !== "cancelled"}<p class="err">{join.error}</p>{/if}
        <button class="primary" data-testid="machines-join" disabled={!codeIn.trim() || rooms.busy} onclick={doJoin}>Join</button>
      {:else}
        <p data-testid="machines-join-net">This code connects over <strong>{netLabel(join.net)}</strong>.</p>
        <p>Check the other machine shows:</p>
        <p class="words" data-testid="machines-join-words">{join.words.join(" · ")}</p>
        <p class="dim">Waiting for the other machine to confirm…</p>
        <button data-testid="machines-join-cancel" onclick={() => act("CancelJoin")}>Cancel</button>
      {/if}

    {:else if mode === "reset"}
      <p>This forgets rooms on this machine: its identity, its keys and its copies of every room. Other machines keep theirs. Use it if you set this machine up the wrong way.</p>
      <label>Type <strong>reset</strong> to confirm <input data-testid="machines-reset-word" bind:value={word} /></label>
      <button class="danger" data-testid="machines-reset-go" disabled={word.trim() !== "reset" || rooms.busy}
        onclick={async () => { await act("ResetRooms"); if (!rooms.error) rooms.machines = null; }}>Reset rooms</button>
    {/if}
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; z-index: 60; background: rgba(0, 0, 0, 0.5); display: flex; align-items: center; justify-content: center; }
  .dlg { width: min(520px, 92vw); max-height: 85vh; overflow-y: auto; background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2);
    padding: var(--sp-3, 12px); display: flex; flex-direction: column; gap: var(--sp-2); color: var(--text-0); font-size: var(--fs-1); }
  .dlg:focus { outline: none; }
  header { display: flex; align-items: center; justify-content: space-between; }
  h3 { margin: 0; font-size: var(--fs-2); }
  p { margin: 0; }
  .row { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
  .nm { font-weight: 600; min-width: 120px; }
  .btns { display: flex; flex-wrap: wrap; gap: var(--sp-1); margin-top: var(--sp-2); }
  .code { display: flex; flex-direction: column; gap: 4px; }
  textarea { font-family: var(--font-mono); font-size: var(--fs-0); }
  label { display: flex; flex-direction: column; gap: 4px; }
  .words { font-family: var(--font-mono); font-size: var(--fs-2); letter-spacing: 0.04em; }
  .icon { background: none; border: 0; color: var(--text-1); cursor: pointer; }
  .dim { color: var(--text-2); }
  .small { font-size: var(--fs-0); }
  .err { color: var(--crit, #d03b3b); }
  .danger { background: var(--crit, #d03b3b); color: #fff; border: 0; border-radius: var(--r-1); padding: 4px 10px; cursor: pointer; }
  .danger:disabled { opacity: 0.5; cursor: not-allowed; }
  .danger-link { background: none; border: 0; color: var(--crit, #d03b3b); cursor: pointer; }
</style>
