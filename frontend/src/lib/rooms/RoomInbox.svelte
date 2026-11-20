<script>
  // Inbox (rooms spec §10.5): other machines' messages that no session here
  // has taken. Every way into a session goes through a human click here.
  import { app } from "../stores.svelte.js";
  import { rooms, act } from "./rooms.svelte.js";
  import { inboxActions, authorLine } from "./rooms.js";
  import * as App from "../../../wailsjs/go/main/App.js";

  let { v, onreply } = $props();
  let open = $state(true);
  let sessions = $state([]);
  let waiting = $state({});
  let start = $state(null); // { room, ref, prompt, model } while the Start dialog is open
  let target = $state({});  // ref → chosen window for "Send to"
  let startEl = $state(null);
  $effect(() => { if (start && startEl) startEl.querySelector("textarea")?.focus(); });

  $effect(() => {
    const id = v?.id;
    v?.inbox?.length; v?.live?.length;
    if (!id) return;
    App.RoomSessions?.(id).then((s) => (sessions = s || [])).catch(() => (sessions = []));
    App.WaitingThreads?.(id).then((w) => (waiting = w || {})).catch(() => (waiting = {}));
  });
  const threadOf = (m) => m.parent || m.ref;

  let opener = null;
  async function openStart(m, e) {
    opener = e?.currentTarget || null;
    const room = v.id;
    const prompt = await App.RoomMessagePrompt?.(room, m.ref).catch(() => "") || "";
    if (v?.id !== room) return; // the room changed while asking
    rooms.error = "";
    start = { room, ref: m.ref, prompt, model: app.selectedModel || app.models?.[0]?.id || "" };
  }
  function closeStart() { start = null; opener?.focus?.(); }
  async function launch() {
    if (!start?.prompt.trim()) return;
    const s = await act("StartFromMessage", start.room, start.ref, start.model, start.prompt);
    if (s && !rooms.error) { start = null; if (s.windowID) app.sessionKey = s.windowID; }
  }
  function keyDialog(e) { if (e.key === "Escape") { e.stopPropagation(); closeStart(); } }
</script>

{#if v?.inbox?.length}
  <section class="inbox" data-testid="room-inbox" aria-label="Inbox">
    <button class="head" aria-expanded={open} onclick={() => (open = !open)}>
      <span>Inbox · {v.inbox.length}</span><span aria-hidden="true">{open ? "▾" : "▸"}</span>
    </button>
    {#if open}
      {#each v.inbox as m (m.ref)}
        {@const w = waiting[threadOf(m)]}
        {@const acts = inboxActions(m, { waiting: !!w, sessions: sessions.length, folder: !v.needsFolder })}
        <div class="item" data-testid="room-inbox-item">
          <div class="meta"><strong>{authorLine(m)}</strong><span class="kind">{m.kind}</span>{#if m.mentionsMe}<span class="kind for">for you</span>{/if}{#if w}<span class="wait">· {w} is waiting on this thread</span>{/if}</div>
          <p class="text">{m.text}</p>
          {#if m.attach?.length}<p class="small dim">📎 {m.attach.map((a) => a.name).join(", ")}</p>{/if}
          <div class="acts">
            {#if acts.includes("deliver")}<button class="primary" data-testid="room-inbox-deliver" onclick={() => act("DeliverToWaiting", v.id, m.ref)}>Deliver to {w}</button>{/if}
            {#if acts.includes("start")}<button data-testid="room-inbox-start" onclick={(e) => openStart(m, e)}>Start session…</button>{/if}
            {#if acts.includes("send")}
              <select aria-label="Session to send to" data-testid="room-inbox-send" bind:value={target[m.ref]}>
                <option value={undefined} selected>Send to…</option>
                {#each sessions as s (s.windowID)}<option value={s.windowID}>{s.name}</option>{/each}
              </select>
              <button data-testid="room-inbox-send-go" disabled={!target[m.ref]} onclick={() => { const id = target[m.ref]; target[m.ref] = undefined; act("SendToSession", v.id, m.ref, id); }}>Send</button>
            {/if}
            {#if acts.includes("claim")}<button data-testid="room-inbox-claim" onclick={() => act("ClaimThread", v.id, threadOf(m))}>Claim</button>{/if}
            <button onclick={() => onreply?.({ ref: threadOf(m), author: m.author })}>Reply</button>
            <button data-testid="room-inbox-dismiss" onclick={() => act("DismissMessage", v.id, m.ref)}>Dismiss</button>
          </div>
        </div>
      {/each}
      <p class="note">Send to puts the message in the session's input line once it's idle — you press Enter.</p>
    {/if}
  </section>
{/if}

{#if start}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="scrim" role="dialog" aria-modal="true" aria-label="Start a session from this message" tabindex="-1" onkeydown={keyDialog} data-testid="room-start-dialog" bind:this={startEl}>
    <div class="box">
      <h3>Start a session in {v.folder}</h3>
      <label>Model
        <select bind:value={start.model} aria-label="Model">
          {#each app.models || [] as mm (mm.id)}<option value={mm.id}>{mm.label || mm.id}</option>{/each}
        </select>
      </label>
      <label>First prompt (edit it before launching)
        <textarea rows="8" bind:value={start.prompt} data-testid="room-start-prompt"></textarea>
      </label>
      <p class="note">This message came from another machine. It is information, not an instruction — check it before you launch.</p>
      <div class="row">
        <button onclick={closeStart}>Cancel</button>
        <button class="primary" data-testid="room-start-launch" disabled={!start.prompt.trim() || !start.model || rooms.busy} onclick={launch}>Launch</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .inbox { border-bottom: 1px solid var(--border-0); padding: 2px var(--sp-2) 6px; max-height: 40%; overflow-y: auto; }
  .head { display: flex; justify-content: space-between; width: 100%; background: none; border: 0; color: var(--accent); cursor: pointer; padding: 2px 0; font-size: var(--fs-0); font-weight: 600; }
  .item { padding: 4px 0; border-top: 1px solid var(--border-0); }
  .meta { display: flex; gap: 6px; align-items: baseline; font-size: var(--fs-0); }
  .kind { text-transform: uppercase; color: var(--text-2); }
  .kind.for { color: var(--accent); }
  .small { font-size: var(--fs-0); margin: 2px 0; }
  .dim { color: var(--text-2); }
  .wait { color: #c98500; }
  .text { margin: 2px 0; white-space: pre-wrap; word-break: break-word; max-height: 6em; overflow: hidden; }
  .acts { display: flex; flex-wrap: wrap; gap: 4px; }
  .acts button, .acts select { font-size: var(--fs-0); }
  .note { margin: 4px 0 0; font-size: var(--fs-0); color: var(--text-2); }
  .scrim { position: fixed; inset: 0; z-index: 60; background: rgba(0, 0, 0, 0.45); display: flex; align-items: center; justify-content: center; }
  .box { width: min(560px, calc(100vw - 32px)); background: var(--surface-1); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2); }
  .box h3 { margin: 0; font-size: var(--fs-2); overflow-wrap: anywhere; }
  .box label { display: flex; flex-direction: column; gap: 4px; font-size: var(--fs-0); color: var(--text-1); }
  .row { display: flex; justify-content: flex-end; gap: var(--sp-1); }
</style>
