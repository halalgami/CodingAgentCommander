<script>
  // Rooms drawer (rooms spec §11). Networking principle (D10): every code and
  // every machine names the network the user chose — Local network or
  // Tailscale — and nothing is preselected.
  import { onMount } from "svelte";
  import Drawer from "../components/Drawer.svelte";
  import { app } from "../stores.svelte.js";
  import { rooms, loadRooms, act } from "./rooms.svelte.js";
  import { deliveryIcon, netLabel, peerLine, outboxGroups, worst } from "./rooms.js";

  onMount(loadRooms);

  let tab = $state("room");
  let name = $state("");
  let codeIn = $state("");
  let joinFolder = $state("");
  let codeNet = $state(""); // explicit choice, no default (D10)
  let codeFor = $state(""); // "pair" | "invite"
  let newName = $state("");
  let kind = $state("note");
  let text = $state("");
  let fields = $state({});
  let replyTo = $state("");

  const st = $derived(rooms.status);
  const v = $derived(rooms.view);
  const inbox = $derived((v?.threads || []).flatMap((t) => [t.root, ...t.replies]).filter((m) => !m.mine && !m.seen));
  const HANDOFF = [["goal", "Goal"], ["context", "Context"], ["branch", "Branch + sha"], ["verify", "How to verify"], ["done", "Done when"]];

  async function pickFolder() { return (await act("PickRoomFolder")) || ""; }
  async function create() {
    const folder = await pickFolder();
    if (!folder) return;
    const r = await act("CreateRoom", folder, newName);
    if (r) { rooms.current = r.id; newName = ""; loadRooms(); }
  }
  async function makeCode() {
    if (!codeNet) { rooms.error = "Choose the network first: Local network or Tailscale."; return; }
    rooms.code = codeFor === "pair" ? await act("PairCode", codeNet) : await act("InviteCode", rooms.current, codeNet);
  }
  async function join() {
    if (!codeIn.trim()) return;
    rooms.code = await act("JoinCode", codeIn.trim(), joinFolder);
  }
  async function post() {
    const f = kind === "handoff" ? fields : kind === "result" ? { outcome: fields.outcome || "" } : null;
    await act("PostRoomMessage", rooms.current, kind, text, replyTo, f);
    if (!rooms.error) { text = ""; fields = {}; replyTo = ""; }
  }
  function open(m) { if (!m.mine && !m.seen) act("MarkRoomSeen", rooms.current, m.ref); }
  function pick(id) { rooms.current = id; loadRooms(); }
</script>

<Drawer title="Rooms" testid="rooms-drawer" wide onclose={() => (app.drawer = null)}>
  {#if rooms.error}<p class="err" role="alert" data-testid="rooms-error">{rooms.error}</p>{/if}

  {#if st?.pending}
    <div class="confirm" data-testid="rooms-confirm">
      <p><strong>{st.pending.name || "A machine"}</strong> wants to {st.pending.kind === "pair" ? "join as your machine" : "join this room"}. Check it shows the same 4 words:</p>
      <p class="words" data-testid="rooms-confirm-words">{st.pending.words.join(" · ")}</p>
      <button class="primary" data-testid="rooms-confirm-yes" onclick={() => act("ConfirmCode", true)}>Words match — allow</button>
      <button data-testid="rooms-confirm-no" onclick={() => act("ConfirmCode", false)}>Decline</button>
    </div>
  {/if}

  {#if !st}
    <p class="dim">Rooms are not available.</p>
  {:else if !st.setUp && !st.joining}
    <section data-testid="rooms-firstrun">
      <h3>Set up rooms</h3>
      <p class="dim">Rooms let your machines and colleagues share a project's messages directly, over the Local network or Tailscale. Nothing goes through a server.</p>
      <label>Your name <input data-testid="rooms-name" bind:value={name} /></label>
      <button class="primary" data-testid="rooms-first" onclick={() => act("RoomsSetup", true, name)}>This is my first Commander machine</button>
      <button data-testid="rooms-pairmode" onclick={() => act("RoomsSetup", false, "")}>Pair with my first machine</button>
    </section>
  {:else if !st.setUp}
    <section data-testid="rooms-join-pair">
      <h3>Pair with your first machine</h3>
      <p class="dim">On your first machine open Rooms → Add my other machine, choose the network, and paste its code here.</p>
      <input placeholder="CMDR-…" data-testid="rooms-code-in" bind:value={codeIn} />
      <button class="primary" data-testid="rooms-join" onclick={join}>Pair</button>
    </section>
  {/if}

  {#if st?.join}
    <div class="joinst" data-testid="rooms-join-status">
      <p data-testid="rooms-join-net">Connecting through <strong>{netLabel(st.join.net)}</strong> (chosen when the code was made).</p>
      <p>Check your other machine shows: <strong class="words" data-testid="rooms-join-words">{st.join.words.join(" · ")}</strong></p>
      <p class="dim">{st.join.status === "waiting" ? "Waiting for the other machine to confirm…" : st.join.status === "done" ? "Done." : st.join.error}</p>
    </div>
  {/if}

  {#if st?.setUp}
    <div class="bar">
      <select aria-label="Room" data-testid="rooms-select" value={rooms.current} onchange={(e) => pick(e.currentTarget.value)}>
        {#each rooms.list as r (r.id)}<option value={r.id}>{r.name}{r.unread ? ` (${r.unread})` : ""}</option>{/each}
        {#if !rooms.list.length}<option value="">No rooms yet</option>{/if}
      </select>
      <input placeholder="New room name (default: folder name)" data-testid="rooms-new-name" bind:value={newName} />
      <button data-testid="rooms-create" onclick={create}>Create room for a folder…</button>
    </div>

    <details class="codes" data-testid="rooms-codes">
      <summary>Add a machine or invite a colleague</summary>
      <div class="netpick" role="radiogroup" aria-label="Network" data-testid="rooms-netpick">
        <span>Connect through:</span>
        <label><input type="radio" name="codenet" value="lan" bind:group={codeNet} /> Local network</label>
        <label class:off={!st.tailscaleUp}><input type="radio" name="codenet" value="tailscale" bind:group={codeNet} disabled={!st.tailscaleUp} /> Tailscale</label>
        {#if !st.tailscaleUp}<span class="dim small">Tailscale is not running on this machine.</span>{/if}
      </div>
      <p class="dim small">Across the internet, both machines need Tailscale.</p>
      {#if st.first}<button data-testid="rooms-pair-code" onclick={() => { codeFor = "pair"; makeCode(); }}>Add my other machine</button>{/if}
      {#if v?.owner}<button data-testid="rooms-invite-code" onclick={() => { codeFor = "invite"; makeCode(); }}>Invite a colleague to {v.name}</button>{/if}
      {#if rooms.code?.code}
        <p>Code ({netLabel(rooms.code.net)}, expires {new Date(rooms.code.expires * 1000).toLocaleString()}):</p>
        <textarea readonly rows="3" data-testid="rooms-code-out">{rooms.code.code}</textarea>
        <button class="link" onclick={() => act("CancelCode")}>Cancel code</button>
      {/if}
      <hr />
      <label>Join a room with an invite code <input placeholder="CMDR-…" data-testid="rooms-invite-in" bind:value={codeIn} /></label>
      <button onclick={async () => (joinFolder = await pickFolder())}>{joinFolder ? joinFolder : "Choose the project folder…"}</button>
      <button data-testid="rooms-invite-join" onclick={join}>Join</button>
    </details>

    {#if v}
      <div class="tabs" role="tablist">
        {#each [["room", "Room"], ["inbox", `Inbox${inbox.length ? ` (${inbox.length})` : ""}`], ["outbox", `Outbox${v.outbox.length ? ` (${v.outbox.length})` : ""}`], ["people", "People & machines"], ["settings", "Settings"]] as [id, label]}
          <button role="tab" aria-selected={tab === id} data-testid={"rooms-tab-" + id} onclick={() => (tab = id)}>{label}</button>
        {/each}
      </div>

      {#if tab === "room"}
        <div class="threads" data-testid="rooms-threads">
          {#each v.threads as t (t.root.ref)}
            <article class="thread" data-testid="rooms-thread">
              {#each [t.root, ...t.replies] as m (m.ref)}
                <div class="msg" class:reply={m !== t.root} class:unseen={!m.mine && !m.seen} data-testid="rooms-msg" onclick={() => open(m)} onkeydown={() => open(m)} role="button" tabindex="0">
                  <span class="kind">{m.kind}</span> <strong>{m.author}</strong> <span class="dim small">{new Date(m.time).toLocaleString()}</span>
                  {#if m.mine}<span class="dlv" data-testid="rooms-delivery" title={deliveryIcon(worst(m.delivery)).label}>{deliveryIcon(worst(m.delivery)).icon}</span>{/if}
                  <p>{m.text}</p>
                  {#if m.fields}{#each Object.entries(m.fields) as [k, val]}{#if val}<p class="small"><span class="dim">{k}:</span> {val}</p>{/if}{/each}{/if}
                </div>
              {/each}
              <button class="link small" onclick={() => (replyTo = t.root.ref)}>Reply</button>
            </article>
          {:else}
            <p class="dim">No messages yet.</p>
          {/each}
        </div>
        <div class="composer" data-testid="rooms-composer">
          {#if replyTo}<p class="small">Replying in thread <button class="link" onclick={() => (replyTo = "")}>cancel</button></p>{/if}
          <select aria-label="Kind" data-testid="rooms-kind" bind:value={kind}>
            {#each ["note", "question", "handoff", "result", "ack"] as k}<option value={k}>{k}</option>{/each}
          </select>
          {#if kind === "handoff"}
            {#each HANDOFF as [k, label]}<input placeholder={label} aria-label={label} bind:value={fields[k]} />{/each}
          {:else if kind === "result"}
            <select aria-label="Outcome" bind:value={fields.outcome}>{#each ["pass", "fail", "partial", "blocked"] as o}<option value={o}>{o}</option>{/each}</select>
          {/if}
          <textarea rows="3" placeholder="Message" data-testid="rooms-text" bind:value={text} maxlength="16000"></textarea>
          <button class="primary" data-testid="rooms-send" disabled={!text.trim() || rooms.busy} onclick={post}>Send</button>
        </div>
      {:else if tab === "inbox"}
        {#each inbox as m (m.ref)}
          <div class="msg unseen" data-testid="rooms-inbox-item"><strong>{m.author}</strong> · {m.kind}<p>{m.text}</p><button data-testid="rooms-mark-seen" onclick={() => open(m)}>Mark seen</button></div>
        {:else}<p class="dim">Nothing new.</p>{/each}
      {:else if tab === "outbox"}
        {#each outboxGroups(v.outbox) as g (g.device)}
          <div data-testid="rooms-outbox-group"><strong>{g.items.length} waiting for {g.name}</strong>
            <ul>{#each g.items as it}<li>{deliveryIcon(it.state).icon} {it.text}</li>{/each}</ul></div>
        {:else}<p class="dim">Everything has reached every machine.</p>{/each}
      {:else if tab === "people"}
        <table class="peers" data-testid="rooms-peers">
          <thead><tr><th scope="col">Machine</th><th scope="col">Network</th><th scope="col">State</th><th scope="col" class="num">Waiting</th></tr></thead>
          <tbody>
            {#each v.peers as p (p.device)}
              <tr data-testid="rooms-peer" data-net={p.net}>
                <td>{p.name}{p.mine ? " (mine)" : ""}</td>
                <td>
                  <select aria-label={"Network for " + p.name} data-testid="rooms-peer-net" value={p.net} onchange={(e) => act("SetDeviceNetwork", p.device, e.currentTarget.value)}>
                    {#if !p.net}<option value="" disabled>Choose…</option>{/if}
                    <option value="lan">Local network</option>
                    <option value="tailscale">Tailscale</option>
                  </select>
                </td>
                <td class="small" title={peerLine(p)}>{p.state}{#if p.reason}<br /><span class="dim">{p.reason}</span>{/if}</td>
                <td class="num">{p.waiting}{#if p.rejected}<br /><span class="err small" title="Entries from this machine that failed checks">{p.rejected} refused</span>{/if}</td>
              </tr>
            {:else}<tr><td colspan="4" class="dim">No other machines in this room yet.</td></tr>{/each}
          </tbody>
        </table>
      {:else if tab === "settings"}
        <p>Folder on this machine: <span class="mono">{v.folder || "not bound"}</span></p>
        <button data-testid="rooms-bind" onclick={async () => { const f = await pickFolder(); if (f) act("BindRoomFolder", rooms.current, f); }}>Choose folder…</button>
      {/if}
    {/if}
  {/if}
</Drawer>

<style>
  .err { color: var(--danger, #d03b3b); }
  .dim { color: var(--text-2); }
  .small { font-size: var(--fs-0); }
  .mono { font-family: var(--font-mono); }
  .words { font-family: var(--font-mono); font-size: var(--fs-2); letter-spacing: 0.04em; }
  .confirm, .joinst { border: 1px solid var(--accent, #3987e5); border-radius: var(--r-2); padding: var(--sp-2); margin-bottom: var(--sp-2); }
  .bar { display: flex; gap: var(--sp-2); flex-wrap: wrap; align-items: center; }
  .codes { margin: var(--sp-2) 0; }
  .netpick { display: flex; gap: var(--sp-2); align-items: center; flex-wrap: wrap; }
  .off { opacity: 0.5; }
  .tabs { display: flex; gap: var(--sp-1); margin: var(--sp-2) 0; flex-wrap: wrap; }
  .tabs [aria-selected="true"] { font-weight: 600; border-bottom: 2px solid var(--accent, #3987e5); }
  .thread { border-bottom: 1px solid var(--border-0); padding: var(--sp-2) 0; }
  .msg { padding: 2px 0; cursor: default; }
  .msg.reply { margin-left: var(--sp-4); }
  .msg.unseen { border-left: 3px solid var(--accent, #3987e5); padding-left: var(--sp-2); }
  .msg p { margin: 2px 0; white-space: pre-wrap; }
  .kind { font-size: var(--fs-0); text-transform: uppercase; color: var(--text-2); }
  .dlv { margin-left: var(--sp-1); }
  .composer { display: flex; flex-direction: column; gap: var(--sp-1); margin-top: var(--sp-2); }
  .peers { width: 100%; border-collapse: collapse; }
  .peers td, .peers th { text-align: left; padding: 4px; border-bottom: 1px solid var(--border-0); }
  .num { text-align: right; }
</style>
