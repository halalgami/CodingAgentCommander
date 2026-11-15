<script>
  import { goHome, isHome } from "../dashboard/dash.svelte.js";
  import {
    app, select, killSession, renameSession, swapSession, enableRemoteControl, ERROR_TTL_MS,
  } from "../stores.svelte.js";
  import { prefs, setPref } from "../prefs.svelte.js";
  import LaunchPanel from "./LaunchPanel.svelte";
  import SessionCard from "./SessionCard.svelte";
  import GroupHeader from "./GroupHeader.svelte";
  import { side, loadLayout, applyLayout } from "../sidebar.svelte.js";
  import {
    UNGROUPED, deriveSections, moveSession, moveGroup, createGroup, renameGroup,
    deleteGroup, setCollapsed, setRemember, sectionOf, materialize,
  } from "../sidebarLayout.js";
  import { dropTarget, DRAG_THRESHOLD } from "../dragReorder.js";
  import { tick, untrack } from "svelte";

  // An optional snippet rendered in the band beneath the session list. The
  // sidebar owns the layout; it deliberately knows nothing about what goes in
  // here, which is what keeps this file free of any feature-specific import.
  let { dock } = $props();

  // The band's height is user-set because it CROPS its content: a flexible
  // height re-crops on every window resize, so the same art sits differently in
  // a small window than a large one. The sidebar still knows nothing about what
  // the band contains — only that its height is worth persisting.
  let aside = $state(null);
  let dockEl = $state(null);
  let dragging = $state(false);

  // Measure the band, never infer it. `flex: 1 0 40%` GROWS past its basis, so
  // computing a height from the basis reported a value well under the real one
  // — and the first arrow-key press then jumped the band smaller instead of
  // nudging it larger.
  const dockHeightNow = () => dockEl?.getBoundingClientRect().height ?? 0;

  function startResize(e) {
    e.preventDefault();
    dragging = true;
    const move = (ev) => {
      if (!aside || !dockEl) return;
      // Measured from the BAND's own bottom edge, not the column's: the aside
      // has padding, so using the column bottom offsets every drag by it.
      const bottom = dockEl.getBoundingClientRect().bottom;
      setPref("dockH", clampDock(bottom - ev.clientY, aside.clientHeight));
    };
    const up = () => {
      dragging = false;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }

  // Both ends are floors, not taste: below MIN the art is unreadable, and above
  // the cap the session list loses the room it needs to stay browsable — which
  // was the point of putting the band under the list in the first place.
  const DOCK_MIN = 140;
  function clampDock(px, columnPx) {
    const max = Math.max(DOCK_MIN, Math.round(columnPx * 0.7));
    return Math.round(Math.min(max, Math.max(DOCK_MIN, px)));
  }

  // Reported to assistive tech, and the same numbers the drag clamps to.
  const dockMax = $derived(Math.max(DOCK_MIN, Math.round((aside?.clientHeight ?? 800) * 0.7)));
  const dockPx = $derived(prefs.dockH > 0 ? prefs.dockH : Math.round(dockHeightNow()));

  // Keyboard parity: a pointer-only resize is unreachable for anyone who cannot
  // drag, and this one persists real state.
  function onHandleKey(e) {
    const step = e.shiftKey ? 40 : 12;
    let delta = 0;
    if (e.key === "ArrowUp") delta = step;
    else if (e.key === "ArrowDown") delta = -step;
    else return;
    e.preventDefault();
    const current = prefs.dockH || dockHeightNow();
    setPref("dockH", clampDock(current + delta, aside?.clientHeight ?? 800));
  }

  // The latch (app.errorMs) attributes to whichever session is selected when
  // it fires — see stores.svelte.js's ERROR_TTL_MS comment — so only the
  // active card can ever be errored, mirroring isActive's own computation.
  // ---- groups ----
  // Every poll replaces app.sessions; re-read the merged layout with it.
  $effect(() => { app.sessions; loadLayout(); });
  const sections = $derived(deriveSections(app.sessions, side.layout, app.stats, app.finished));
  const cwdOf = (id) => app.stats[id]?.cwd ?? "";
  const cwdMap = () => Object.fromEntries(app.sessions.map((s) => [s.windowID, cwdOf(s.windowID)]));
  const selected = $derived(app.sessionKey.split(":")[0]);

  // A session selected from anywhere (dashboard, palette, notification) must
  // be visible: open its group if it is collapsed.
  let lastSelected = "";
  $effect(() => {
    const id = selected;
    untrack(() => {
      if (!id || id === lastSelected) return;
      lastSelected = id;
      const key = sectionOf(side.layout, id);
      const g = side.layout.groups.find((x) => x.id === key);
      if (g?.collapsed) applyLayout(setCollapsed(side.layout, key, false));
    });
  });

  let live = $state("");
  const say = (msg) => { live = ""; queueMicrotask(() => (live = msg)); };
  const nameOf = (id) => app.sessions.find((s) => s.windowID === id)?.name ?? id;
  const keyName = (key) => key === UNGROUPED ? "Ungrouped" : (side.layout.groups.find((g) => g.id === key)?.name ?? key);

  // Edits start from what is on screen, not the stored layout, which may not
  // list a window yet (see materialize).
  const base = () => materialize(side.layout, sections);

  function move(id, toKey, index) {
    applyLayout(moveSession(base(), id, toKey, index, cwdOf(id), cwdMap()));
    say(`Moved ${nameOf(id)} to ${keyName(toKey)}, position ${index + 1}`);
  }
  // A move can re-create the card under another section's #each, which drops
  // focus to <body>; put it back so a keyboard user can keep moving.
  async function refocus(sel) {
    await tick();
    listEl?.querySelector(sel)?.focus();
  }

  // Alt+Up/Down: within the visible section, then across the section edge.
  function keyMove(id, dir) {
    const i = sections.findIndex((s) => s.sessions.some((x) => x.windowID === id));
    if (i < 0) return;
    const pos = sections[i].sessions.findIndex((x) => x.windowID === id);
    const target = pos + dir;
    if (target >= 0 && target < sections[i].sessions.length) move(id, sections[i].key, target);
    else {
      const next = sections[i + dir];
      if (!next) return;
      move(id, next.key, dir > 0 ? 0 : next.sessions.length);
    }
    refocus(`[data-window-id="${id}"] [data-testid="session-name"]`);
  }
  function groupKeyMove(gid, dir) {
    const gi = side.layout.groups.findIndex((g) => g.id === gid);
    if (gi + dir < 0 || gi + dir >= side.layout.groups.length) return;
    applyLayout(moveGroup(side.layout, gid, gi + dir));
    say(`Moved group ${keyName(gid)} to position ${gi + dir + 1}`);
    refocus(`[data-group-id="${gid}"] [data-testid="group-toggle"]`);
  }

  let newName = $state("");
  let naming = $state(null); // { windowID } while the new-group field is open
  function startNewGroup(windowID = null) { naming = { windowID }; newName = ""; ctx = null; }
  function commitNewGroup() {
    if (!naming) return;
    const n = newName.trim();
    if (n) {
      const { layout } = createGroup(base(), n, naming.windowID, naming.windowID ? cwdOf(naming.windowID) : "");
      applyLayout(layout);
      say(`Created group ${n}`);
    }
    naming = null;
  }

  // Card context menu: Move to group.
  let ctx = $state(null); // { x, y, windowID }
  function openCtx(e, windowID) { ctx = { x: e.clientX, y: e.clientY, windowID }; }
  $effect(() => {
    if (!ctx) return;
    const k = (e) => { if (e.key === "Escape") { e.stopPropagation(); ctx = null; } };
    const d = (e) => { if (!e.target.closest || !e.target.closest(".ctx")) ctx = null; };
    window.addEventListener("keydown", k, true);
    window.addEventListener("pointerdown", d, true);
    return () => { window.removeEventListener("keydown", k, true); window.removeEventListener("pointerdown", d, true); };
  });
  function ctxMove(toKey) {
    const id = ctx.windowID; ctx = null;
    move(id, toKey, (base().order[toKey] ?? []).length);
  }
  function collapseOthers(gid) {
    let l = side.layout;
    for (const g of l.groups) l = setCollapsed(l, g.id, g.id !== gid);
    applyLayout(l);
  }

  // ---- drag (pointer events only: HTML5 DnD would reach the file-drop router) ----
  let listEl = $state(null);
  let drag = $state(null);   // { kind: "card"|"group", id, active, y }
  let marker = $state(null); // { top } insertion line, list-relative

  function rects() {
    return [...listEl.querySelectorAll("[data-drop-kind]")].map((el) => {
      const b = el.getBoundingClientRect();
      return { kind: el.dataset.dropKind, key: el.dataset.dropKey ?? el.closest("[data-drop-key]")?.dataset.dropKey,
        windowID: el.dataset.windowId, top: b.top, bottom: b.bottom };
    }).map((r) => r.kind === "card" ? { ...r, key: sectionOf(side.layout, r.windowID) } : r);
  }

  function beginDrag(e, kind, id) {
    if (e.button !== 0 || e.target.closest("input, .actions, .gmenu-wrap, .menu")) return;
    const x0 = e.clientX, y0 = e.clientY;
    const el = e.currentTarget?.closest?.("li, .ghead") ?? null;
    drag = { kind, id, active: false, y: y0 };
    let scrollTimer = null;
    const autoscroll = () => {
      if (!drag?.active || !listEl) return;
      const b = listEl.getBoundingClientRect();
      if (drag.y < b.top + 24) listEl.scrollTop -= 8;
      else if (drag.y > b.bottom - 24) listEl.scrollTop += 8;
      updateMarker();
    };
    const move = (ev) => {
      if (!drag) return;
      drag.y = ev.clientY;
      if (!drag.active && Math.hypot(ev.clientX - x0, ev.clientY - y0) >= DRAG_THRESHOLD) {
        drag.active = true;
        side.dragging = true;
        el?.classList.add("lifted");
        scrollTimer = setInterval(autoscroll, 30);
      }
      if (drag.active) updateMarker();
    };
    const finish = (commit) => {
      window.removeEventListener("pointermove", move, true);
      window.removeEventListener("pointerup", up, true);
      window.removeEventListener("pointercancel", cancel, true);
      window.removeEventListener("keydown", key, true);
      clearInterval(scrollTimer);
      const d = drag;
      drag = null; marker = null; side.dragging = false;
      el?.classList.remove("lifted");
      if (!d?.active) return;
      // The click that follows a drag must not select / toggle anything.
      // Swallow the click this gesture produces, if any — but never a later,
      // unrelated one (released outside the window, or an engine that sends
      // no click): the blocker is disarmed on the next tick either way.
      const block = (c) => { c.stopPropagation(); c.preventDefault(); };
      window.addEventListener("click", block, { capture: true, once: true });
      setTimeout(() => window.removeEventListener("click", block, { capture: true }), 0);
      if (commit) drop(d);
    };
    const up = () => finish(true);
    const cancel = () => finish(false);
    const key = (ev) => { if (ev.key === "Escape" && drag?.active) { ev.stopPropagation(); finish(false); } };
    window.addEventListener("pointermove", move, true);
    window.addEventListener("pointerup", up, true);
    window.addEventListener("pointercancel", cancel, true);
    window.addEventListener("keydown", key, true);
  }

  function updateMarker() {
    if (!drag?.active || !listEl) return;
    const rs = rects();
    const t = dropTarget(rs, drag.y, drag.kind, drag.id);
    const base = listEl.getBoundingClientRect().top - listEl.scrollTop;
    if (!t) { marker = null; return; }
    if (t.newGroup) {
      const n = rs.find((r) => r.kind === "new");
      marker = n ? { top: n.top - base, zone: true } : null;
      return;
    }
    if (drag.kind === "group") {
      const hs = rs.filter((r) => r.kind === "header" && r.key !== UNGROUPED && r.key !== drag.id);
      const at = hs[t.groupIndex];
      const last = rs.filter((r) => r.kind === "card" || r.kind === "header").at(-1);
      marker = { top: (at ? at.top : (last?.bottom ?? 0)) - base };
      return;
    }
    const cards = rs.filter((r) => r.kind === "card" && r.key === t.key && r.windowID !== drag.id);
    const header = rs.find((r) => r.kind === "header" && r.key === t.key);
    let top;
    if (t.index === Infinity) top = (cards.at(-1)?.bottom ?? header?.bottom ?? 0);
    else if (cards[t.index]) top = cards[t.index].top;
    else top = cards.at(-1)?.bottom ?? header?.bottom ?? 0;
    marker = { top: top - base };
  }

  function drop(d) {
    const t = dropTarget(rects(), d.y, d.kind, d.id);
    if (!t) return;
    if (d.kind === "group") {
      const gi = side.layout.groups.findIndex((g) => g.id === d.id);
      if (gi !== t.groupIndex) { applyLayout(moveGroup(side.layout, d.id, t.groupIndex)); say(`Moved group ${keyName(d.id)} to position ${t.groupIndex + 1}`); }
      return;
    }
    if (t.newGroup) return startNewGroup(d.id);
    const idx = t.index === Infinity ? (base().order[t.key] ?? []).filter((x) => x !== d.id).length : t.index;
    move(d.id, t.key, idx);
  }

  const startCardDrag = (e, id) => beginDrag(e, "card", id);
  const startGroupDrag = (e, id) => beginDrag(e, "group", id);

  function isErrored(windowID) {
    return app.errorMs > 0 && Date.now() - app.errorMs < ERROR_TTL_MS &&
      app.sessionKey.split(":")[0] === windowID;
  }
</script>

<aside data-testid="sidebar" bind:this={aside}>
  <LaunchPanel />
  <button class="dash-entry" class:on={isHome()} data-testid="sidebar-dashboard" onclick={goHome}>▦ Dashboard</button>
  <ul class="sessions" data-testid="session-list" bind:this={listEl} class:dragging-list={drag?.active}>
    {#each sections as sec (sec.key)}
      {#if sec.group}
        <li class="gli"><GroupHeader section={sec}
          ontoggle={(id, c) => applyLayout(setCollapsed(side.layout, id, c))}
          onrename={(id, n) => applyLayout(renameGroup(side.layout, id, n))}
          ondelete={(id) => applyLayout(deleteGroup(base(), id))}
          onremember={(id, r) => applyLayout(setRemember(side.layout, id, r))}
          oncollapseothers={collapseOthers}
          onkeymove={groupKeyMove}
          onpointerdown={startGroupDrag} /></li>
      {:else}
        <li class="gli ung" data-testid="ungrouped-header" data-drop-key={UNGROUPED} data-drop-kind="header"
          class:hidden={!side.layout.groups.length}>Ungrouped</li>
      {/if}
      {#if !sec.group?.collapsed}
        {#each sec.sessions as s (s.windowID)}
          <SessionCard
            session={s}
            stat={app.stats[s.windowID]}
            isActive={selected === s.windowID}
            isFinished={!!app.finished[s.windowID]}
            isErrored={isErrored(s.windowID)}
            models={app.models}
            onselect={select}
            onrename={renameSession}
            onkill={killSession}
            onswap={swapSession}
            onrc={enableRemoteControl}
            oncontext={openCtx}
            onkeymove={keyMove}
            onpointerdown={startCardDrag}
          />
        {/each}
      {/if}
    {/each}
    {#if marker}<li class="ins" class:zone={marker.zone} style:top={marker.top + "px"} aria-hidden="true"></li>{/if}
    <li class="gli">
      {#if naming}
        <!-- svelte-ignore a11y_autofocus -->
        <input class="newg" data-testid="new-group-input" placeholder="Group name" bind:value={newName} autofocus
          onkeydown={(e) => { if (e.key === "Enter") commitNewGroup(); if (e.key === "Escape") naming = null; }}
          onblur={commitNewGroup} />
      {:else}
        <button class="newg-btn" data-testid="new-group" data-drop-kind="new" onclick={() => startNewGroup()}>+ New group</button>
      {/if}
    </li>
  </ul>
  <span class="sr" aria-live="polite">{live}</span>
  {#if ctx}
    <div class="ctx" role="menu" data-testid="card-menu" style:left={ctx.x + "px"} style:top={ctx.y + "px"}>
      <div class="ctx-h">Move to group</div>
      {#each side.layout.groups as g (g.id)}
        {#if sectionOf(side.layout, ctx.windowID) !== g.id}
          <button role="menuitem" onclick={() => ctxMove(g.id)}>{g.name}</button>
        {/if}
      {/each}
      {#if sectionOf(side.layout, ctx.windowID) !== UNGROUPED}
        <button role="menuitem" onclick={() => ctxMove(UNGROUPED)}>Ungrouped</button>
      {/if}
      <button role="menuitem" data-testid="card-menu-new" onclick={() => startNewGroup(ctx.windowID)}>New group…</button>
    </div>
  {/if}
  {#if dock}
    <!-- The WAI-ARIA "Window Splitter" pattern: a FOCUSABLE separator carrying
         its own value, adjusted with the arrow keys. svelte-ignore is warranted
         because the rule assumes every separator is decorative, which is the
         case this pattern is the documented exception to. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="dock-handle" data-testid="sidebar-dock-handle"
      class:dragging
      role="separator" aria-orientation="horizontal" aria-label="Resize the panel below the session list"
      aria-valuenow={dockPx} aria-valuemin={DOCK_MIN} aria-valuemax={dockMax}
      tabindex="0" onpointerdown={startResize} onkeydown={onHandleKey}
    ></div>
  {/if}
  <div
    class="dock" data-testid="sidebar-dock" bind:this={dockEl}
    style:flex-basis={prefs.dockH > 0 ? prefs.dockH + "px" : null}
    style:flex-grow={prefs.dockH > 0 ? "0" : null}
  >{@render dock?.()}</div>
</aside>

<style>
  aside {
    width: var(--sidebar-w); flex: none; height: 100%;
    background: var(--surface-1); border-right: 1px solid var(--border-0);
    padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3);
    /* The aside must CLIP, not scroll. It used to be `overflow-y: auto`, which
       made the whole column one scroller — with a dock at the bottom that means
       the dock scrolls out of view instead of staying put. */
    overflow: hidden;
    min-height: 0;
  }
  /* flex: 0 1 auto — grow no further than the content, shrink freely.
     min-height: 0 is REQUIRED: the default `min-height: auto` on a flex item
     refuses to shrink below its content, so the list would push the dock off the
     bottom and its own scrollbar would never appear. */
  .sessions {
    flex: 0 1 auto; min-height: 0; overflow-y: auto;
    /* margin-bottom: auto pins the band to the column's BOTTOM edge. Nothing
       here grows once dockH is set (the list is grow-0 by design above, and the
       inline dockH sets flex-grow: 0), so in a short column the leftover space
       used to collect BELOW the dock and the band floated mid-column. That also
       broke the drag: startResize measures from the band's own bottom edge, so
       shrinking the band moved that edge up and the next pointermove subtracted
       from a smaller number — the band raced upward away from the cursor
       instead of handing the space to the list. With a full list there is no
       free space and auto resolves to 0, so the crowded case is unchanged. */
    margin: 0 0 auto; padding: 0;
  }
  /* flex: 1 0 40% — take the remainder, and never shrink below 40% of the
     sidebar height however many sessions exist. That is the cap §4.1 asks for,
     expressed as the dock's floor rather than the list's ceiling, so a sidebar
     with three sessions still gives the list all the room it wants. */
  .dock { flex: 1 0 40%; min-height: 0; display: flex; }
  /* No snippet passed (the public build passes none) -> no band at all, and the
     list gets the whole column exactly as it did before. */
  .dock:empty { display: none; }
  /* Never taller than 70% of the column, whatever is stored: a persisted height
     from a large window must not squeeze the session list to nothing when the
     same prefs are read back in a small one. */
  .dock { max-height: 70%; }

  .dock-handle {
    flex: none; height: 8px; margin: calc(var(--sp-2) * -1) 0 calc(var(--sp-2) * -1);
    cursor: row-resize; background: transparent; border-radius: var(--r-1);
    transition: background var(--t-fast) var(--ease-out);
  }
  .dock-handle:hover, .dock-handle.dragging, .dock-handle:focus-visible {
    background: var(--accent-dim);
  }
  .dock-handle:focus-visible { outline: 1px solid var(--accent); outline-offset: 1px; }
  /* No band, no handle: the {#if dock} guard above is what enforces this — the
     public build passes no dock snippet, so neither element is rendered. */
  .gli { list-style: none; }
  .sessions { position: relative; }
  .dragging-list { cursor: grabbing; user-select: none; }
  .sessions :global(.lifted) { opacity: 0.45; }
  .ins { position: absolute; left: 0; right: 0; height: 2px; margin-top: -1px; background: var(--accent); border-radius: 1px; pointer-events: none; list-style: none; }
  .ins.zone { height: 28px; background: var(--accent-faint); border: 1px dashed var(--accent); }
  .ung { font-size: var(--fs-0, 11px); color: var(--text-2); text-transform: uppercase; letter-spacing: .05em; margin: var(--sp-2) 2px 2px; }
  .ung.hidden { display: none; }
  .newg-btn {
    width: 100%; margin-top: var(--sp-2); background: none; border: 1px dashed var(--border-0); border-radius: var(--r-2);
    color: var(--text-2); padding: 5px 8px; cursor: pointer; font-size: var(--fs-1); text-align: left;
  }
  .newg-btn:hover, .newg-btn:focus-visible { color: var(--text-0); border-color: var(--accent-dim); }
  .newg { width: 100%; margin-top: var(--sp-2); font-size: var(--fs-1); }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  .ctx {
    position: fixed; z-index: 50; display: flex; flex-direction: column; min-width: 180px;
    background: var(--surface-2); border: 1px solid var(--border-1); border-radius: var(--r-2); padding: 4px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }
  .ctx-h { font-size: 11px; color: var(--text-2); padding: 4px 8px; }
  .ctx button { background: none; border: 0; color: var(--text-0); text-align: left; padding: 6px 8px; border-radius: var(--r-1); cursor: pointer; font-size: var(--fs-1); }
  .ctx button:hover, .ctx button:focus-visible { background: var(--accent-faint); }
  .dash-entry {
    display: block; width: calc(100% - 2 * var(--sp-2)); margin: var(--sp-2); text-align: left;
    background: var(--surface-1); border: 1px solid var(--border-0); border-radius: var(--r-2);
    color: var(--text-1); padding: 6px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  .dash-entry:hover, .dash-entry.on { color: var(--text-0); border-color: var(--accent-dim); }
</style>
