<script>
  // Docked room panel (UX spec §6): beside the terminal, never on top of it
  // unless the window is too narrow. One page: people, feed, composer.
  import { tick } from "svelte";
  import { app } from "../stores.svelte.js";
  import { prefs, setPref } from "../prefs.svelte.js";
  import { rooms, act, summaryOf, closePanel, projectOf } from "./rooms.svelte.js";
  import { netLabel, chatOrder, deliveryLine, panelMode, revealLabel, SEEN_MS, promptOf, handoffBranch } from "./rooms.js";
  import { toast } from "../stores.svelte.js";
  import * as App from "../../../wailsjs/go/main/App.js";
  import CloseRoomDialog from "./CloseRoomDialog.svelte";
  import RoomLive from "./RoomLive.svelte";
  import RoomInbox from "./RoomInbox.svelte";
  import RoomSettings from "./RoomSettings.svelte";
  import { byteLen, baseName, fmtSize, authorLine, canSend, outboxGroups } from "./rooms.js";

  const sum = $derived(summaryOf(rooms.current));
  const v = $derived(rooms.view && rooms.view.id === rooms.current ? rooms.view : null);
  const feed = $derived(chatOrder(v?.threads));
  const here = revealLabel() === "Show in Finder" ? "this Mac" : "this PC";
  const selCwd = $derived(app.stats[app.sessionKey.split(":")[0]]?.cwd || "");
  const selUnshared = $derived(!!selCwd && !projectOf(selCwd)?.roomId);

  // ---- width, dock or overlay ----
  let winW = $state(window.innerWidth);
  $effect(() => {
    const r = () => (winW = window.innerWidth);
    window.addEventListener("resize", r);
    return () => window.removeEventListener("resize", r);
  });
  const width = $derived(Math.max(300, prefs.roomPanelW || 360));
  const mode = $derived(panelMode(winW / ((prefs.uiScale || 100) / 100), prefs.sidebarW, width));
  function startResize(e) {
    e.preventDefault();
    const x0 = e.clientX, w0 = width;
    const move = (ev) => setPref("roomPanelW", Math.min(640, Math.max(300, Math.round(w0 + (x0 - ev.clientX)))));
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }
  function resizeKey(e) {
    const d = { ArrowLeft: 16, ArrowRight: -16 }[e.key];
    if (!d) return;
    e.preventDefault();
    setPref("roomPanelW", Math.min(640, Math.max(300, width + d)));
  }

  // ---- menu and dialogs ----
  let menu = $state(false);
  let dialog = $state(null); // "close" | "leave" | "remove"
  let settingsOpen = $state(false);
  // Claims per thread root (rooms spec §8.3).
  const claims = $derived(Object.fromEntries((v?.threads || []).map((t) => [t.root.ref, t])));
  const bytes = $derived(byteLen(text));
  $effect(() => {
    if (!menu) return;
    const k = (e) => { if (e.key === "Escape") { e.stopPropagation(); menu = false; } };
    const d = (e) => { if (!e.target.closest?.(".menu-wrap")) menu = false; };
    window.addEventListener("keydown", k, true);
    window.addEventListener("pointerdown", d, true);
    return () => { window.removeEventListener("keydown", k, true); window.removeEventListener("pointerdown", d, true); };
  });
  // ---- rename (owner, open room) ----
  let renaming = $state(false);
  let newName = $state("");
  function startRename() { menu = false; newName = sum?.name || ""; renaming = true; }
  // Escape removes the input, which can fire blur: cancelled must stay cancelled,
  // and Enter + blur must not send twice (final review M1).
  let renameBusy = false;
  async function rename() {
    if (!renaming || renameBusy) return;
    if (!newName.trim() || newName.trim() === sum?.name) { renaming = false; return; }
    renameBusy = true;
    await act("RenameRoom", rooms.current, newName.trim());
    renameBusy = false;
    if (!rooms.error) renaming = false;
  }
  function renameKey(e) {
    if (e.key === "Enter") { e.preventDefault(); rename(); }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); renaming = false; }
  }

  async function copyPrompt(m) {
    try { await navigator.clipboard.writeText(promptOf(m, sum?.name || "room")); toast("Copied as a prompt."); }
    catch { toast("Couldn't copy to the clipboard.", "error"); }
  }

  async function chooseFolder() {
    menu = false;
    const f = await act("PickRoomFolder");
    if (f) await act("BindRoomFolder", rooms.current, f);
  }

  // ---- folder suggestions for a room that arrived here without one ----
  let suggest = $state([]);
  $effect(() => {
    const id = rooms.current, need = sum?.needsFolder;
    suggest = [];
    if (id && need) App.SuggestFolders?.(id).then((s) => (suggest = s || [])).catch(() => {});
  });

  // ---- seen: ≥ SEEN_MS on screen while the window is focused (UX §6.3) ----
  let feedEl = $state(null);
  // Per room: switching rooms must never carry a ref (or its timer) across
  // (final review I2).
  const visibleSince = new Map();
  const sent = new Set();
  let seenRoom = "";
  $effect(() => {
    if (!feedEl) return;
    const roomId = rooms.current;
    if (roomId !== seenRoom) { visibleSince.clear(); sent.clear(); seenRoom = roomId; }
    // 60% of the message — or 60% of the feed for one taller than the feed (final review I3).
    const io = new IntersectionObserver((es) => {
      for (const e of es) {
        const ref = e.target.dataset.ref;
        const root = e.rootBounds?.height || 0;
        const vis = e.isIntersecting && (e.intersectionRatio >= 0.6 || (root > 0 && e.intersectionRect.height >= 0.6 * root));
        if (vis) { if (!visibleSince.has(ref)) visibleSince.set(ref, 0); }
        else visibleSince.delete(ref);
      }
    }, { root: feedEl, threshold: [0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.8, 1] });
    feed.items; // re-observe when the feed changes
    tick().then(() => feedEl?.querySelectorAll("[data-unseen='true']").forEach((n) => io.observe(n)));
    const iv = setInterval(() => {
      const focused = document.hasFocus();
      for (const [ref, since] of visibleSince) {
        if (!focused) { visibleSince.set(ref, 0); continue; }
        if (!since) { visibleSince.set(ref, Date.now()); continue; }
        if (Date.now() - since >= SEEN_MS && !sent.has(ref)) {
          sent.add(ref);
          if (roomId === rooms.current) act("MarkRoomSeen", roomId, ref);
        }
      }
    }, 250);
    return () => { io.disconnect(); clearInterval(iv); };
  });
  // Keep the newest message in view.
  $effect(() => { feed.items.length; tick().then(() => feedEl && (feedEl.scrollTop = feedEl.scrollHeight)); });

  // ---- composer ----
  const KINDS = [["note", "Note"], ["question", "Question"], ["handoff", "Handoff"], ["result", "Result"]];
  const HANDOFF = [["goal", "Goal"], ["context", "Context"], ["branch", "Branch + sha"], ["verify", "How to verify"], ["done", "Done when"]];
  let kind = $state("note");
  let text = $state("");
  let fields = $state({});
  let replyTo = $state(null);
  let files = $state([]); // paths the user picked (≤ 8)
  let mentions = $state([]); // device ids
  let outboxOpen = $state(false);
  async function pickFiles() {
    try {
      const got = await App.PickRoomFiles();
      files = [...new Set([...files, ...(got || [])])].slice(0, 8);
    } catch (e) { toast(String(e?.message || e)); }
  }
  const toggleMention = (d) => (mentions = mentions.includes(d) ? mentions.filter((x) => x !== d) : [...mentions, d]);
  async function saveAttachment(sha) {
    try {
      const p = await App.SaveRoomAttachment(rooms.current, sha);
      toast("Saved to " + p);
    } catch (e) { toast(String(e?.message || e)); }
  }
  // Handoff: branch + sha from the selected session, when it is in this room's
  // project (spec §6.4) — once per (room, session) handoff, so a cleared field
  // stays cleared, and a late answer for another room or session is dropped.
  let prefilled = "";
  $effect(() => {
    if (kind !== "handoff") { prefilled = ""; return; }
    const wid = app.sessionKey.split(":")[0], room = rooms.current;
    const key = room + "|" + wid;
    if (!wid || prefilled === key || projectOf(selCwd)?.roomId !== room) return;
    prefilled = key;
    App.GitHead?.(wid).then((h) => {
      if (kind === "handoff" && rooms.current === room && app.sessionKey.split(":")[0] === wid && !fields.branch) fields.branch = handoffBranch(h);
    }).catch(() => {});
  });
  // A draft belongs to its room.
  let draftRoom = rooms.current;
  $effect(() => { if (rooms.current !== draftRoom) { draftRoom = rooms.current; fields = {}; replyTo = null; prefilled = ""; files = []; mentions = []; } });
  async function send() {
    if (!canSend(text, files, bytes, rooms.busy)) return;
    const f = kind === "handoff" ? fields : kind === "result" ? { outcome: fields.outcome || "pass" } : null;
    await act("SendRoomPost", rooms.current, { kind, text, parent: replyTo?.ref || "", fields: f, files, mentions });
    if (!rooms.error) { text = ""; fields = {}; replyTo = null; kind = "note"; files = []; mentions = []; }
  }
  function key(e) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey || !e.shiftKey)) { e.preventDefault(); send(); }
  }
  const rootOf = (m) => (m.parent || m.ref);
  const ack = (m) => act("PostRoomMessage", rooms.current, "ack", "👍", rootOf(m), null);
  const closedLine = $derived(sum?.closed
    ? (sum.closedBy === "you left" ? "You left this room." : `Closed by ${sum.closedBy || "the owner"}${sum.closedAt ? " on " + new Date(sum.closedAt).toLocaleDateString() : ""}.`)
    : "");
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (Esc anywhere in the panel returns focus to the terminal) -->
<aside class="panel" class:overlay={mode === "overlay"} data-testid="room-panel" data-mode={mode} aria-label="Room" style:width={width + "px"}
  onkeydown={(e) => { if (e.key === "Escape" && !menu && !dialog) { e.stopPropagation(); document.querySelector(".xterm-helper-textarea")?.focus(); } }}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (WAI-ARIA window splitter) -->
  <div class="grip" role="separator" aria-orientation="vertical" aria-label="Room panel width" tabindex="0"
    aria-valuenow={width} aria-valuemin="300" aria-valuemax="640" onpointerdown={startResize} onkeydown={resizeKey}></div>

  <header>
    {#if renaming}
      <!-- svelte-ignore a11y_autofocus (the user just chose Rename) -->
      <input class="name" data-testid="room-rename-input" aria-label="Room name" maxlength="80" bind:value={newName} onkeydown={renameKey} onblur={rename} autofocus />
    {:else}
      <strong class="name" data-testid="room-panel-name">{sum?.name || "Rooms"}</strong>
    {/if}
    {#if sum}
      <span class="menu-wrap">
        <button class="icon" data-testid="room-menu" aria-label="Room options" aria-haspopup="menu" aria-expanded={menu} onclick={() => (menu = !menu)}>⋯</button>
        {#if menu}
          <span class="menu" role="menu">
            {#if sum.owner && !sum.closed}<button role="menuitem" data-testid="room-menu-rename" onclick={startRename}>Rename…</button>{/if}
            <button role="menuitem" onclick={chooseFolder}>Folder on this machine…</button>
            {#if sum.owner && !sum.closed}<button role="menuitem" data-testid="room-menu-invite" onclick={() => { menu = false; rooms.machines = "invite"; }}>Invite colleague…</button>{/if}
            <button role="menuitem" onclick={() => { menu = false; rooms.machines = "list"; }}>Machines &amp; network…</button>
            <button role="menuitem" data-testid="room-menu-settings" onclick={() => { menu = false; settingsOpen = true; }}>Settings on this machine…</button>
            {#if sum.owner && !sum.closed}<button role="menuitem" data-testid="room-menu-close" onclick={() => { menu = false; dialog = "close"; }}>Close room…</button>{/if}
            {#if !sum.owner && !sum.closed}<button role="menuitem" data-testid="room-menu-leave" onclick={() => { menu = false; dialog = "leave"; }}>Leave room…</button>{/if}
            <button role="menuitem" data-testid="room-menu-remove" onclick={() => { menu = false; dialog = "remove"; }}>Remove from this machine…</button>
          </span>
        {/if}
      </span>
    {/if}
    <button class="icon" aria-label="Close room panel" data-testid="room-panel-close" onclick={closePanel}>✕</button>
  </header>

  {#if rooms.error}<p class="err" role="alert">{rooms.error}</p>{/if}

  {#if !sum}
    <p class="dim pad">No room open. Share a project from its session card, or pick a room under Rooms in the title bar.</p>
  {:else}
    {#if v?.peers?.length}
      <div class="people" data-testid="room-people">
        {#each v.peers as p (p.device)}
          <span class="chip" class:refused={!!p.tried || !p.net} data-testid="room-chip" title={p.reason || p.state}>
            <span class="dot" class:on={p.state?.startsWith("direct")} aria-hidden="true"></span>
            {p.name} · {p.net ? netLabel(p.net) : "No network chosen"}
            {#if p.tried}
              <button class="link" data-testid="room-chip-switch" onclick={() => act("SetDeviceNetwork", p.device, p.tried)}>Use {netLabel(p.tried)} instead</button>
            {:else if !p.net}
              <select aria-label={"Network for " + p.name} onchange={(e) => act("SetDeviceNetwork", p.device, e.currentTarget.value)}>
                <option value="" selected disabled>Choose…</option>
                <option value="lan">Local network</option>
                <option value="tailscale">Tailscale</option>
              </select>
            {/if}
            {#if p.reason}<span class="why">{p.reason}</span>{/if}
          </span>
        {/each}
      </div>
    {:else}
      <p class="dim pad small">No other machines yet — your other Commander machines will see this room once they're paired. <button class="link" onclick={() => (rooms.machines = "list")}>Machines &amp; network…</button></p>
    {/if}

    <RoomLive live={v?.live || []} />

    {#if v?.drafts?.length}
      <div class="banner" data-testid="room-drafts">
        <p>{v.drafts.length} agent post{v.drafts.length === 1 ? "" : "s"} held — this machine reached its hourly limit.</p>
        {#each v.drafts as d (d.id)}
          <div class="draft"><span class="small drafttext">{d.kind}: {d.text}</span>
            <button class="link small" data-testid="room-draft-release" onclick={() => act("ReleaseDraft", rooms.current, d.id)}>Post</button>
            <button class="link small" onclick={() => act("DropDraft", rooms.current, d.id)}>Drop</button></div>
        {/each}
      </div>
    {/if}

    {#if !sum.closed}<RoomInbox {v} onreply={(r) => (replyTo = r)} />{/if}

    {#if selUnshared}
      <p class="hint small">This session's project isn't shared — <button class="link" onclick={() => (rooms.share = { windowID: app.sessionKey.split(":")[0], cwd: selCwd, label: projectOf(selCwd)?.label || selCwd.split(/[\\/]/).pop() })}>Share</button></p>
    {/if}

    {#if sum.needsFolder}
      <div class="banner" data-testid="room-needs-folder">
        <p>Choose this project's folder on this machine.</p>
        {#each suggest as s (s.path)}
          <button class="primary" data-testid="room-suggest" onclick={() => act("BindRoomFolder", rooms.current, s.path)}>Use {s.path} {s.source === "session" ? "(open now, same repository)" : "(same repository)"}</button>
        {/each}
        <button onclick={chooseFolder}>Choose another…</button>
      </div>
    {/if}

    {#if v?.outbox?.length}
      <div class="outbox" data-testid="room-outbox">
        <button class="link small" aria-expanded={outboxOpen} onclick={() => (outboxOpen = !outboxOpen)}>Outbox · {v.outbox.length} waiting {outboxOpen ? "▾" : "▸"}</button>
        {#if outboxOpen}{#each outboxGroups(v.outbox) as g (g.device)}
          <p class="small"><strong>{g.name}</strong> — {g.items.length} not there yet</p>
          {#each g.items as it (it.ref + it.device)}<p class="small dim ob">{it.state}: {it.text}</p>{/each}
        {/each}{/if}
      </div>
    {/if}

    {#if (sum.queued || 0) >= 2}<p class="amber small">{sum.queued} messages are only on {here} — they'll go when another machine connects.</p>{/if}

    <div class="feed" bind:this={feedEl} data-testid="room-feed">
      {#each feed.items as it, i (it.msg.ref)}
        {#if i === feed.firstUnread}<div class="divider" data-testid="room-new-divider"><span>New</span></div>{/if}
        {@const dl = deliveryLine(it.msg, v?.peers, here)}
        {@const th = it.depth === 0 ? claims[it.msg.ref] : null}
        <div class="msg" class:reply={it.depth > 0} class:mine={it.msg.mine} data-ref={it.msg.ref} data-unseen={!it.msg.mine && !it.msg.seen} data-testid="room-msg">
          <div class="meta"><strong>{authorLine(it.msg)}</strong>{#if it.msg.kind !== "note"}<span class="kind">{it.msg.kind}</span>{/if}<span class="dim small">{new Date(it.msg.time).toLocaleString()}</span>
            {#if th?.conflict}<span class="claim conflict" data-testid="room-claim-conflict">Claimed twice: {th.claimedBy.join(", ")}</span>
            {:else if th?.claimedBy?.length}<span class="claim" data-testid="room-claimed">Claimed by {th.claimedBy.join(", ")}</span>{/if}
            {#if it.msg.mentionsMe}<span class="claim mention" data-testid="room-mention-me">for you</span>{:else if it.msg.mentions?.length}<span class="dim small">@ {it.msg.mentions.join(", ")}</span>{/if}</div>
          {#if it.msg.text}<p>{it.msg.text}</p>{/if}
          {#if it.msg.attach?.length}
            <div class="atts">{#each it.msg.attach as at (at.sha256)}
              <button class="att" data-testid="room-attachment" title={"Save to the project's .commander/attachments folder" + (at.have ? "" : " (fetches it from another machine)")} onclick={() => saveAttachment(at.sha256)}>📎 {at.name} <span class="dim">{fmtSize(at.size)}</span></button>
            {/each}</div>
          {/if}
          {#if th?.outOfBudget}<p class="small amber" data-testid="room-budget">Automatic deliveries used up on this thread. <button class="link" data-testid="room-grant" onclick={() => act("GrantThread", rooms.current, it.msg.ref)}>Allow 5 more</button></p>{/if}
          {#if it.msg.fields}{#each Object.entries(it.msg.fields) as [k, val]}{#if val}<p class="small"><span class="dim">{k}:</span> {val}</p>{/if}{/each}{/if}
          {#if dl.text}<p class="small dl" data-testid="room-delivery" data-amber={dl.amber}>{dl.text}</p>{/if}
          {#if sum.closed}
            <span class="acts"><button class="link small" data-testid="room-copy-prompt" onclick={() => copyPrompt(it.msg)}>Copy as prompt</button></span>
          {:else}
            <span class="acts">
              <button class="link small" onclick={() => (replyTo = { ref: rootOf(it.msg), author: it.msg.author })}>Reply</button>
              <button class="link small" data-testid="room-copy-prompt" onclick={() => copyPrompt(it.msg)}>Copy as prompt</button>
              {#if !it.msg.mine && it.msg.kind !== "ack"}<button class="link small" aria-label="Acknowledge" onclick={() => ack(it.msg)}>👍</button>{/if}
              {#if th?.myClaim}<button class="link small" data-testid="room-release" onclick={() => act("ReleaseThread", rooms.current, it.msg.ref)}>Release</button>
              {:else if th && (it.msg.kind === "handoff" || it.msg.kind === "question")}<button class="link small" data-testid="room-claim" onclick={() => act("ClaimThread", rooms.current, it.msg.ref)}>Claim</button>{/if}
            </span>
          {/if}
        </div>
      {:else}
        <p class="dim pad">No messages yet. Say what you're working on, or hand off a task.</p>
      {/each}
    </div>

    {#if sum.closed}
      <p class="closed" data-testid="room-closed-note">{closedLine} Messages stay readable; nobody can post.</p>
    {:else}
      <div class="composer" data-testid="room-composer">
        {#if replyTo}<p class="small">Replying to {replyTo.author} <button class="link" onclick={() => (replyTo = null)}>cancel</button></p>{/if}
        <div class="kinds" role="radiogroup" aria-label="Message kind">
          {#each KINDS as [k, label]}
            <button role="radio" aria-checked={kind === k} data-testid={"room-kind-" + k} onclick={() => (kind = k)}>{label}</button>
          {/each}
        </div>
        {#if kind === "handoff"}
          {#each HANDOFF as [k, label]}<input placeholder={label} aria-label={label} bind:value={fields[k]} />{/each}
        {:else if kind === "result"}
          <select aria-label="Outcome" bind:value={fields.outcome}>{#each ["pass", "fail", "partial", "blocked"] as o}<option value={o}>{o}</option>{/each}</select>
        {/if}
        {#if (kind === "question" || kind === "handoff") && v?.peers?.length}
          <div class="kinds" aria-label="Notify machines">
            <span class="dim small">Notify</span>
            {#each v.peers as p (p.device)}
              <button role="checkbox" aria-checked={mentions.includes(p.device)} data-testid="room-mention" onclick={() => toggleMention(p.device)}>{p.name}</button>
            {/each}
          </div>
        {/if}
        <textarea rows="3" placeholder="Message — Enter sends, Shift+Enter new line" data-testid="room-text" bind:value={text} onkeydown={key}></textarea>
        {#if files.length}
          <div class="atts" data-testid="room-files">{#each files as f (f)}<span class="att">📎 {baseName(f)} <button class="link" aria-label={"Remove " + baseName(f)} onclick={() => (files = files.filter((x) => x !== f))}>×</button></span>{/each}</div>
        {/if}
        {#if bytes > 12000}<p class="small" class:over={bytes > 16384} data-testid="room-bytes">{(bytes / 1024).toFixed(1)} / 16 KB</p>{/if}
        <div class="row">
          <button data-testid="room-attach" disabled={files.length >= 8} onclick={pickFiles} title="Attach files (up to 8, 2 MB each)">Attach…</button>
          <button class="primary" data-testid="room-send" disabled={!canSend(text, files, bytes, rooms.busy)} onclick={send}>Send</button>
        </div>
      </div>
    {/if}
  {/if}

  {#if settingsOpen && v}<RoomSettings {v} onclose={() => (settingsOpen = false)} />{/if}
  {#if dialog && sum}<CloseRoomDialog mode={dialog} {sum} peers={v?.peers || []} onclose={() => (dialog = null)} />{/if}
</aside>

<style>
  .panel {
    position: relative; flex: none; display: flex; flex-direction: column; min-width: 300px; height: 100%;
    border-left: 1px solid var(--border-0); background: var(--surface-1); color: var(--text-0); font-size: var(--fs-1);
  }
  .panel.overlay { position: absolute; right: 0; top: 0; bottom: 0; z-index: 25; box-shadow: -8px 0 24px rgba(0, 0, 0, 0.35); }
  .grip { position: absolute; left: -3px; top: 0; bottom: 0; width: 6px; cursor: col-resize; z-index: 2; }
  .grip:hover, .grip:focus-visible { background: var(--accent-dim); }
  header { display: flex; align-items: center; gap: var(--sp-1); padding: var(--sp-2); border-bottom: 1px solid var(--border-0); }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .icon { background: none; border: 0; color: var(--text-1); cursor: pointer; padding: 2px 6px; border-radius: var(--r-1); }
  .icon:hover, .icon:focus-visible { background: var(--surface-3); color: var(--text-0); }
  .menu-wrap { position: relative; }
  .menu {
    position: absolute; right: 0; top: 100%; z-index: 30; display: flex; flex-direction: column; min-width: 220px;
    background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: 4px; box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }
  .menu button { background: none; border: 0; color: var(--text-0); text-align: left; padding: 6px 8px; border-radius: var(--r-1); cursor: pointer; }
  .menu button:hover, .menu button:focus-visible { background: var(--accent-faint); }
  .people { display: flex; flex-wrap: wrap; gap: 4px; padding: var(--sp-2); border-bottom: 1px solid var(--border-0); }
  .chip { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 4px; padding: 2px 8px; border-radius: 10px; background: var(--surface-2); font-size: var(--fs-0); }
  .chip.refused { outline: 1px solid #c98500; }
  .chip .why { flex-basis: 100%; color: var(--text-2); }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--text-2); }
  .dot.on { background: var(--ok, #2fa84f); }
  .pad { padding: var(--sp-2); margin: 0; }
  .hint { margin: 0; padding: 4px var(--sp-2); color: var(--text-2); }
  .banner { margin: var(--sp-2); padding: var(--sp-2); border: 1px solid #c98500; border-radius: var(--r-2); display: flex; flex-direction: column; gap: 4px; }
  .banner p { margin: 0; }
  .amber { color: #c98500; margin: 0; padding: 4px var(--sp-2); }
  .feed { flex: 1; overflow-y: auto; padding: var(--sp-2); display: flex; flex-direction: column; gap: var(--sp-2); }
  .divider { display: flex; align-items: center; gap: 6px; color: var(--accent); font-size: var(--fs-0); }
  .divider::before, .divider::after { content: ""; flex: 1; border-top: 1px solid var(--accent); }
  .msg { position: relative; }
  .msg.reply { margin-left: var(--sp-4); padding-left: var(--sp-2); border-left: 2px solid var(--border-1); }
  .msg[data-unseen="true"] { border-left: 3px solid var(--accent); padding-left: var(--sp-2); }
  .msg p { margin: 2px 0; white-space: pre-wrap; word-break: break-word; }
  .meta { display: flex; gap: 6px; align-items: baseline; }
  .kind { font-size: var(--fs-0); text-transform: uppercase; color: var(--text-2); }
  .dl { color: var(--text-2); }
  .dl[data-amber="true"] { color: #c98500; }
  .acts { display: none; gap: 6px; }
  .msg:hover .acts, .msg:focus-within .acts { display: flex; }
  .composer { display: flex; flex-direction: column; gap: 4px; padding: var(--sp-2); border-top: 1px solid var(--border-0); }
  .kinds { display: flex; gap: 4px; }
  .kinds button { background: var(--surface-2); border: 1px solid var(--border-0); border-radius: 10px; padding: 1px 8px; cursor: pointer; color: var(--text-1); font-size: var(--fs-0); }
  .kinds button[aria-checked="true"] { border-color: var(--accent); color: var(--text-0); }
  .closed { margin: 0; padding: var(--sp-2); border-top: 1px solid var(--border-0); color: var(--text-2); }
  .link { background: none; border: 0; color: var(--accent); cursor: pointer; padding: 0; }
  .dim { color: var(--text-2); }
  .small { font-size: var(--fs-0); }
  .err { color: var(--crit, #d03b3b); margin: 0; padding: 4px var(--sp-2); }
  .claim { font-size: var(--fs-0); color: var(--text-2); border: 1px solid var(--border-1); border-radius: 8px; padding: 0 6px; }
  .claim.conflict { color: #c98500; border-color: #c98500; }
  .draft { display: flex; gap: 6px; align-items: baseline; }
  .drafttext { white-space: pre-wrap; word-break: break-word; max-height: 12em; overflow-y: auto; flex: 1; }
  .over { color: var(--crit, #d03b3b); }
  .atts { display: flex; flex-wrap: wrap; gap: 4px; }
  .att { display: inline-flex; gap: 4px; align-items: baseline; background: var(--surface-2); border: 1px solid var(--border-0); border-radius: 8px; padding: 1px 8px; font-size: var(--fs-0); color: var(--text-0); cursor: pointer; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .claim.mention { color: var(--accent); border-color: var(--accent); }
  .row { display: flex; gap: 6px; justify-content: flex-end; }
  .outbox { padding: 4px var(--sp-2); border-bottom: 1px solid var(--border-0); }
  .outbox p { margin: 2px 0; }
  .ob { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .kinds button[role="checkbox"][aria-checked="true"] { border-color: var(--accent); color: var(--text-0); }
</style>
