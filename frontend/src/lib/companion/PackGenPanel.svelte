<script>
  // The pack generator, as a NON-MODAL panel.
  //
  // Non-modal is a requirement, not a style choice: a run takes minutes and
  // costs money, and the user must be able to close this, keep working, and
  // come back. There is deliberately no backdrop and no aria-modal — the
  // sidebar and terminal stay clickable throughout. Closing never cancels.
  import {
    gen, currentPhase, closePackGen, conditionsFor, selectedSlots, togglePick,
    setStaging, syncModel, activeProvider, refreshCost, pickBase, saveKey,
    clearKey, startRun, cancelRun, saveRun, discardRun,
    promptFor, setPromptFor, regenerateVariant, nudgeFocus, selectPack,
    exportPack, importPack, askDeletePack, cancelDeletePack, confirmDeletePack,
    cancelOverwrite, addScenes, toggleAddPick, setAddStaging,
  } from "./packgen.svelte.js";
  import { PHASES, slotKey, blockerFor, statusText, costLine, addableScenes, runHoldsArt, selectedAddSlots } from "./packgen-logic.js";

  const PHASE_LABEL = { base: "Portrait", slots: "Scenes", generate: "Generating", review: "Review" };

  // The browser is a TAB, not a phase: it looks at the pack already saved,
  // which has nothing to do with where a run has got to.
  let tab = $state("create");
  const saved = $derived(gen.browse);
  const fmtDate = (s) => (s ? new Date(s).toLocaleString() : "");

  let phase = $derived(currentPhase());
  let provider = $derived(activeProvider());
  let picked = $derived(selectedSlots());
  let run = $derived(gen.run);

  // A disabled button with no explanation is the most common way a wizard
  // dead-ends, so this is a sentence, not a boolean. The rules mirror
  // validatePackGenSlots so the user is told while choosing rather than
  // refused after pressing Generate.
  let blocker = $derived(blockerFor({
    name: gen.name, base: gen.base, provider, model: gen.model,
    picks: gen.picks, run: gen.run, conditions: gen.conditions,
  }));
  let cost = $derived(costLine(picked.length, gen.cost));
  // One image's price, for the regenerate button. Shown only when known: a
  // stale or absent price rendered as $0.00 reads as free.
  let selectedModel = $derived(provider?.models?.find((m) => m.id === gen.model) ?? null);
  let modelNote = $derived(selectedModel?.note ?? "");
  let modelRefusesCostly = $derived(selectedModel?.refusal === "black_image");

  let costOne = $derived.by(() => {
    const per = provider?.models?.find((m) => m.id === gen.model);
    return per?.known ? `$${per.usd.toFixed(2)}` : "";
  });

  // Rows for the Add-scenes picker: every slot/condition, flagged present.
  let addRows = $derived(addableScenes(gen.slotDefaults, gen.conditions, saved));
  // The count/label/guard must reflect what addScenes will actually SEND, not
  // the raw picks: a present-slot pick left over from a switched pack (or one
  // that's since become present) is filtered out by selectedAddSlots and must
  // not be counted here either, or the button lies about what pressing it does.
  let addSelected = $derived(
    selectedAddSlots(
      gen.addPicks,
      new Set(addRows.filter((r) => r.present).map((r) => slotKey(r.slot, r.when))),
    ),
  );
  let addCount = $derived(addSelected.length);
  let addCostLine = $derived(costLine(addCount, gen.addCost));
  // Why "Add" is unavailable, as a sentence (mirrors the Create-tab blocker).
  let addBlocker = $derived.by(() => {
    if (!provider) return "No image provider is available.";
    if (!provider.hasKey) return `Add an API key for ${provider.id}.`;
    if (gen.run?.status === "running") return "A pack is already generating.";
    if (gen.run && !gen.run.saved && runHoldsArt(gen.run)) return "Save or discard the previous images first.";
    if (addCount === 0) return "Tick a scene to add.";
    return "";
  });
</script>

{#if gen.open}
  <aside class="panel" data-testid="packgen-panel" aria-label="Create a companion pack">
    <header>
      <h2>CREATE A PACK</h2>
      <button class="close" data-testid="packgen-close" onclick={closePackGen} title="Close">✕</button>
    </header>

    <nav class="tabs" data-testid="packgen-tabs">
      <button class:on={tab === "create"} data-testid="packgen-tab-create"
        onclick={() => (tab = "create")}>Create</button>
      <button class:on={tab === "browse"} data-testid="packgen-tab-browse"
        onclick={() => (tab = "browse")}>
        Saved pack{saved?.variants?.length ? ` (${saved.variants.length})` : ""}
      </button>
    </nav>

    {#if tab === "browse"}
      <div class="body" data-testid="packgen-browse">
        {#if gen.error}<p class="err" data-testid="packgen-error">{gen.error}</p>{/if}
        <!-- The library comes FIRST: with several packs, "which one am I
             looking at" has to be answerable before anything below makes
             sense. Shown even for a single pack, so the answer never changes
             shape as packs are added. -->
        <div class="libactions">
          <button class="ghost" data-testid="packgen-import" onclick={importPack}>
            Import a pack…
          </button>
          <button class="ghost" data-testid="packgen-export"
            disabled={!saved} onclick={exportPack}>
            Export this pack…
          </button>
        </div>

        {#if gen.library.length}
          <div class="library" data-testid="packgen-library">
            {#each gen.library as p (p.path)}
              <div class="libcell">
              <button
                class="lib" class:on={p.active}
                data-testid="packgen-lib-{p.folder}"
                title={p.path}
                onclick={() => !p.active && selectPack(p)}
              >
                {#if p.thumb}
                  <img src="/media/pack/{p.thumb}" alt="" />
                {:else}
                  <span class="nothumb" aria-hidden="true"></span>
                {/if}
                <span class="libname">{p.name}</span>
                <span class="dim">{p.scenes} scene{p.scenes === 1 ? "" : "s"}</span>
                {#if p.active}<span class="badge">active</span>{/if}
                {#if p.warnings}<span class="warnbadge" title="{p.warnings} warning(s)">!</span>{/if}
              </button>
              <!-- Outside the select button: nesting it would make every
                   delete click also select the pack. -->
              <button
                class="del" title="Delete {p.name}"
                data-testid="packgen-del-{p.folder}"
                onclick={(e) => { e.stopPropagation(); askDeletePack(p); }}>✕</button>
              </div>
            {/each}
          </div>
        {/if}

        {#if !saved}
          <p class="dim">No pack saved yet. Generate one on the Create tab.</p>
        {:else if saved.missing}
          <!-- Configured but gone: the folder was moved or deleted outside the
               app. Saying that is very different from showing an empty pack,
               which reads as "your art vanished". -->
          <p class="warn" data-testid="packgen-missing">
            This pack’s folder is no longer at <span class="mono">{saved.path}</span>.
            It was moved or deleted outside Commander. Pick another pack below,
            or import a backup.
          </p>
        {:else}
          <p class="dim">
            <b>{saved.name}</b> — {(saved.variants ?? []).length} scene{(saved.variants ?? []).length === 1 ? "" : "s"}.
            {#if !saved.hasBase}
              This pack has no stored portrait, so single scenes cannot be
              regenerated from it — only replaced by generating a new pack.
            {/if}
          </p>
          {#each saved.variants ?? [] as v (v.slot + "|" + v.when + "|" + (v.mood ?? ""))}
            {@const key = v.slot + "|" + v.when}
            {@const open = gen.openVariant === key}
            <section class="saved" data-testid="packgen-saved-{v.slot}{v.when ? '-' + v.when : ''}">
              <button class="savedhead" onclick={() => (gen.openVariant = open ? "" : key)}>
                <img src="/media/pack/{v.preview}" alt="" style:object-position="{v.focusX * 100}% {v.focusY * 100}%" />
                <span class="meta">
                  <b>{v.slot}</b>{v.when ? ` · ${v.when}` : ""}
                  <span class="dim">{fmtDate(v.generatedAt) || "no generation record"}</span>
                </span>
                <span class="chev">{open ? "▾" : "▸"}</span>
              </button>
              {#if open}
                <div class="detail">
                  <!-- Cheap where regenerating costs an image: the model
                       composes each frame independently, so one scene sits
                       off-centre and another does not. -->
                  <label class="field">
                    Horizontal crop <span class="mono">{v.focusX.toFixed(2)}</span>
                    <input type="range" min="0" max="1" step="0.01" value={v.focusX}
                      data-testid="packgen-focusx-{v.slot}"
                      onchange={(e) => nudgeFocus(v, +e.target.value, v.focusY)} />
                  </label>
                  <label class="field">
                    Vertical crop <span class="mono">{v.focusY.toFixed(2)}</span>
                    <input type="range" min="0" max="1" step="0.01" value={v.focusY}
                      data-testid="packgen-focusy-{v.slot}"
                      onchange={(e) => nudgeFocus(v, v.focusX, +e.target.value)} />
                  </label>

                  {#if v.prompt}
                    <label class="field">
                      Prompt <span class="dim">sent exactly as shown</span>
                      <textarea rows="5" data-testid="packgen-prompt-{v.slot}"
                        value={promptFor(v)}
                        oninput={(e) => setPromptFor(v, e.target.value)}></textarea>
                    </label>
                  {:else}
                    <p class="dim">No prompt was recorded for this image.</p>
                  {/if}
                  {#if v.model}<p class="dim mono">{v.model}{v.seed ? ` · seed ${v.seed}` : ""}</p>{/if}

                  <div class="actions">
                    <button class="primary" data-testid="packgen-regen-{v.slot}"
                      disabled={!saved.hasBase || gen.busy || gen.run?.status === "running"}
                      onclick={() => { regenerateVariant(v); tab = "create"; }}>
                      Regenerate this scene{costOne ? ` — ${costOne}` : ""}
                    </button>
                  </div>
                </div>
              {/if}
            </section>
          {/each}

          {#if saved.hasBase}
            <section class="addscenes" data-testid="packgen-add">
              <h3>Add scenes</h3>
              <p class="dim">
                Add a scene that isn’t in this pack yet — including one that
                failed when the pack was first made. Scenes already here are
                marked and can be re-rolled from their own row above.
              </p>
              {#each addRows as r (slotKey(r.slot, r.when))}
                {@const key = slotKey(r.slot, r.when)}
                {@const pick = gen.addPicks[key]}
                <div class="addrow" data-testid="packgen-add-{r.slot}{r.when ? '-' + r.when : ''}">
                  <label class="addhead">
                    <input
                      type="checkbox"
                      checked={r.present ? true : (pick?.on ?? false)}
                      disabled={r.present}
                      onchange={(e) => toggleAddPick(r.slot, r.when, e.target.checked)} />
                    <b>{r.label}</b>
                    {#if r.present}<span class="dim">in pack</span>{/if}
                  </label>
                  {#if !r.present && pick?.on}
                    <textarea rows="2"
                      data-testid="packgen-add-staging-{r.slot}{r.when ? '-' + r.when : ''}"
                      value={pick.staging}
                      oninput={(e) => setAddStaging(r.slot, r.when, e.target.value)}></textarea>
                  {/if}
                </div>
              {/each}

              <div class="summary" data-testid="packgen-add-summary">
                <b>{addCostLine.images}</b>
                {#if addCostLine.price}· about <b>{addCostLine.price}</b>{/if}
                {#if addCostLine.note}<span class="dim">· {addCostLine.note}</span>{/if}
              </div>
              {#if addBlocker}
                <p class="blocker" data-testid="packgen-add-blocker">{addBlocker}</p>
              {/if}
              <div class="actions">
                <button class="primary" data-testid="packgen-add-go"
                  disabled={!!addBlocker || gen.busy}
                  onclick={() => { addScenes(); tab = "create"; }}>
                  {gen.busy ? "Starting…" : `Add ${addCount} scene${addCount === 1 ? "" : "s"}`}
                </button>
              </div>
            </section>
          {/if}
        {/if}
      </div>
    {:else}

    <!-- The step rail is a progress indicator, not navigation: the last two
         phases belong to the run, and clicking back into the form while images
         are being paid for is not a thing to offer. -->
    <ol class="rail" data-testid="packgen-rail">
      {#each PHASES as p, i (p)}
        <li class:done={PHASES.indexOf(phase) > i} class:now={phase === p} data-testid="packgen-step-{p}">
          <span class="n">{i + 1}</span>{PHASE_LABEL[p]}
        </li>
      {/each}
    </ol>

    <div class="body">
      {#if gen.error}
        <p class="err" data-testid="packgen-error">{gen.error}</p>
      {/if}

      {#if phase === "base"}
        <label class="field">
          Pack name
          <input
            data-testid="packgen-name" placeholder="e.g. Wafa"
            bind:value={gen.name} />
        </label>

        <div class="field">
          Base portrait
          <div class="baserow">
            <button class="ghost" data-testid="packgen-pick-base" onclick={pickBase}>
              {gen.base ? "Change…" : "Choose an image…"}
            </button>
            {#if gen.base}
              <span class="dim mono" data-testid="packgen-base-dims">
                {gen.base.path.split("/").pop()} · {gen.base.w}×{gen.base.h}
              </span>
            {/if}
          </div>
          {#if gen.base?.warning}
            <p class="warn" data-testid="packgen-base-warning">{gen.base.warning}</p>
          {/if}
          <p class="dim">
            Every scene is generated from this one image, so it is what holds the
            character together. A clear, well-lit portrait works best.
          </p>
        </div>

        <label class="field">
          Provider
          <select
            data-testid="packgen-provider" bind:value={gen.providerID}
            onchange={() => { syncModel(); refreshCost(); }}>
            {#each gen.providers as p (p.id)}
              <option value={p.id}>{p.id}{p.hasKey ? "" : " — no key"}</option>
            {/each}
          </select>
        </label>

        {#if provider && !provider.hasKey}
          <div class="field keybox" data-testid="packgen-key-form">
            <span>API key for {provider.id}</span>
            <input
              type="password" data-testid="packgen-key-input"
              placeholder="paste your key" bind:value={gen.keyInput} />
            <button class="primary" data-testid="packgen-key-save" onclick={saveKey}>Save key</button>
            <p class="dim">
              Stored in your operating system's keychain. It is sent to
              {provider.id} and nowhere else, and never leaves Go.
            </p>
          </div>
        {:else if provider}
          <div class="field keyrow">
            <span class="dim">Key saved for {provider.id}</span>
            <button class="ghost" data-testid="packgen-key-clear" onclick={clearKey}>Remove</button>
          </div>
        {/if}

        <label class="field">
          Model
          <select data-testid="packgen-model" bind:value={gen.model} onchange={refreshCost}>
            {#each provider?.models ?? [] as m (m.id)}
              <option value={m.id}>
                {m.label}{m.known ? ` — $${m.usd.toFixed(2)}/image` : ""}
              </option>
            {/each}
          </select>
          <!-- A measured caveat, not marketing. The cheap model returns a
               billed black frame on roughly one image in six, and the user
               should learn that before the run rather than from the bill. -->
          {#if modelNote}
            <p class="dim" data-testid="packgen-model-note"
              class:warn={modelRefusesCostly}>{modelNote}</p>
          {/if}
        </label>

        <label class="field">
          Style
          <select data-testid="packgen-style" bind:value={gen.styleID}>
            {#each gen.styles as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
          </select>
          <p class="dim">
            {gen.styles.find((s) => s.id === gen.styleID)?.text ?? ""}
          </p>
        </label>

        <div class="actions">
          <button class="primary" data-testid="packgen-to-slots" onclick={() => (gen.step = "slots")}>
            Next — choose scenes
          </button>
        </div>

      {:else if phase === "slots"}
        <p class="dim">
          Each scene is one image. The idle scene is required; the rest are
          optional. You can edit what happens in each one.
        </p>

        {#each gen.slotDefaults as s (s.slot)}
          {@const key = slotKey(s.slot, "")}
          {@const pick = gen.picks[key]}
          <section class="slot" data-testid="packgen-slot-{s.slot}">
            <label class="slothead">
              <input
                type="checkbox" data-testid="packgen-slot-{s.slot}-on"
                checked={pick?.on ?? false}
                disabled={s.slot === "idle"}
                onchange={(e) => togglePick(s.slot, "", e.target.checked)} />
              <b>{s.slot}</b>
              {#if s.slot === "idle"}<span class="dim">required</span>{/if}
            </label>
            {#if pick?.on}
              <textarea
                data-testid="packgen-slot-{s.slot}-staging" rows="3"
                value={pick.staging}
                oninput={(e) => setStaging(s.slot, "", e.target.value)}></textarea>
              {#if conditionsFor(s.slot).length}
                <div class="conds">
                  {#each conditionsFor(s.slot) as c (c.name)}
                    {@const ck = slotKey(s.slot, c.name)}
                    <label class="cond" data-testid="packgen-cond-{s.slot}-{c.name}">
                      <input
                        type="checkbox" checked={gen.picks[ck]?.on ?? false}
                        onchange={(e) => togglePick(s.slot, c.name, e.target.checked)} />
                      +{c.name}
                    </label>
                  {/each}
                </div>
              {/if}
            {/if}
          </section>
        {/each}

        <div class="summary" data-testid="packgen-summary">
          <b>{cost.images}</b>
          {#if cost.price}· about <b>{cost.price}</b>{/if}
          {#if cost.note}<span class="dim">· {cost.note}</span>{/if}
        </div>

        {#if blocker}
          <p class="blocker" data-testid="packgen-blocker">{blocker}</p>
        {/if}
        <div class="actions">
          <button class="ghost" data-testid="packgen-back" onclick={() => (gen.step = "base")}>Back</button>
          <button
            class="primary" data-testid="packgen-start"
            disabled={!!blocker || gen.busy} onclick={startRun}>
            {gen.busy ? "Starting…" : `Generate ${picked.length}`}
          </button>
        </div>

      {:else}
        <!-- generate and review share the grid; only the footer differs, so
             they are one branch rather than two near-identical copies. -->
        <div class="runline" data-testid="packgen-runline">
          <span><b>{run?.done ?? 0}</b> of {run?.total ?? 0} done</span>
          <span class="dim">· {run?.billed ?? 0} billed</span>
          {#if run?.status && run.status !== "running"}
            <span class="dim">· {run.status}</span>
          {/if}
        </div>
        {#if run?.error}
          <p class="err" data-testid="packgen-run-error">{run.error}</p>
        {/if}

        <div class="grid" data-testid="packgen-grid">
          {#each run?.items ?? [] as item, i (slotKey(item.slot, item.when) + i)}
            <figure
              class="card {item.status}"
              data-testid="packgen-card-{slotKey(item.slot, item.when)}">
              {#if item.preview}
                <img src="/media/pack/{item.preview}" alt="{item.slot} scene" />
              {:else}
                <div class="ph" class:pulse={item.status === "running"}></div>
              {/if}
              <figcaption>
                <b>{item.slot}</b>{item.when ? ` · ${item.when}` : ""}
                {#if statusText(item)}
                  <span class="st" title={item.error}>{statusText(item)}</span>
                {/if}
              </figcaption>
            </figure>
          {/each}
        </div>

        <div class="actions">
          {#if phase === "generate"}
            <button class="ghost" data-testid="packgen-cancel" onclick={cancelRun}>
              Stop after this image
            </button>
            <span class="dim">You can close this panel; generating continues.</span>
          {:else}
            <button class="ghost" data-testid="packgen-discard" disabled={gen.busy} onclick={discardRun}>
              Discard
            </button>
            <button class="primary" data-testid="packgen-save" disabled={gen.busy} onclick={saveRun}>
              {gen.busy ? "Saving…" : "Save & use this pack"}
            </button>
          {/if}
        </div>
      {/if}
    </div>
    {/if}
  </aside>

  {#if gen.confirmOverwrite}
    <div class="confirm-backdrop" data-testid="packgen-overwrite-backdrop"
      onclick={cancelOverwrite} aria-hidden="true"></div>
    <div class="confirm" role="alertdialog" aria-modal="true"
      aria-label="Replace the existing pack" data-testid="packgen-overwrite-confirm"
      onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); cancelOverwrite(); } }}
      tabindex="-1" {@attach (el) => { el.focus(); }}>
      <h3>Replace the existing pack?</h3>
      <p>{gen.confirmOverwrite}</p>
      <p class="dim">Export it first if you want a copy you can restore.</p>
      <div class="actions">
        <button class="ghost" data-testid="packgen-overwrite-cancel"
          onclick={cancelOverwrite}>Pick another name</button>
        <button class="danger" data-testid="packgen-overwrite-go"
          disabled={gen.busy} onclick={() => startRun({ overwrite: true })}>
          {gen.busy ? "Generating…" : "Replace it"}
        </button>
      </div>
    </div>
  {/if}

  {#if gen.confirmDelete}
    <div class="confirm-backdrop" data-testid="packgen-delete-backdrop"
      onclick={cancelDeletePack} aria-hidden="true"></div>
    <div class="confirm" role="alertdialog" aria-modal="true"
      aria-label="Delete this pack" data-testid="packgen-delete-confirm"
      onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); cancelDeletePack(); } }}
      tabindex="-1"
      {@attach (el) => { el.focus(); }}>
      <h3>Delete “{gen.confirmDelete.name}”?</h3>
      <p>
        This permanently removes {gen.confirmDelete.scenes}
        image{gen.confirmDelete.scenes === 1 ? "" : "s"} and cannot be undone.
        {#if gen.confirmDelete.active}
          It is the pack currently in use, so the companion will have none until
          you pick another.
        {/if}
      </p>
      <p class="dim mono">{gen.confirmDelete.path}</p>
      <p class="dim">Export it first if you want a copy you can restore.</p>
      <div class="actions">
        <button class="ghost" data-testid="packgen-delete-cancel"
          onclick={cancelDeletePack}>Keep it</button>
        <button class="danger" data-testid="packgen-delete-go"
          disabled={gen.busy} onclick={confirmDeletePack}>
          {gen.busy ? "Deleting…" : "Delete permanently"}
        </button>
      </div>
    </div>
  {/if}
{/if}

<style>
  /* No backdrop: the panel is non-modal by design, so the rest of the deck
     stays reachable while a run is in flight. */
  .panel {
    position: fixed; top: var(--titlebar-h); right: 0; bottom: 0; width: 560px;
    max-width: 100vw;
    background: var(--surface-1); border-left: 1px solid var(--border-0);
    z-index: var(--layer-drawer); display: flex; flex-direction: column;
    animation: slide var(--t-med) var(--ease-out);
    box-shadow: -8px 0 24px oklch(0% 0 0 / 0.25);
  }
  @keyframes slide { from { transform: translateX(24px); opacity: 0; } }
  header {
    display: flex; align-items: center; justify-content: space-between;
    padding: var(--sp-3) var(--sp-4); border-bottom: 1px solid var(--border-0);
  }
  h2 { margin: 0; font-size: var(--fs-3); font-weight: 600; letter-spacing: 0.06em; }
  .close { background: none; border: 0; color: var(--text-1); cursor: pointer; font-size: var(--fs-2); }
  .close:hover { color: var(--text-0); }

  .rail {
    display: flex; gap: var(--sp-2); list-style: none; margin: 0;
    padding: var(--sp-2) var(--sp-4); border-bottom: 1px solid var(--border-0);
    font-size: var(--fs-1); color: var(--text-2);
  }
  .rail li { display: flex; align-items: center; gap: 6px; }
  .rail .n {
    display: inline-grid; place-items: center; width: 18px; height: 18px;
    border-radius: 50%; border: 1px solid var(--border-0); font-size: 10px;
  }
  .rail li.now { color: var(--text-0); }
  .rail li.now .n { border-color: var(--accent); color: var(--accent); }
  .rail li.done .n { background: var(--accent-dim); border-color: var(--accent-dim); }

  .tabs { display: flex; gap: 2px; padding: 0 var(--sp-4); border-bottom: 1px solid var(--border-0); }
  .tabs button {
    background: none; border: 0; border-bottom: 2px solid transparent;
    color: var(--text-2); cursor: pointer; font: inherit; font-size: var(--fs-1);
    padding: 6px 10px;
  }
  .tabs button:hover { color: var(--text-0); }
  .tabs button.on { color: var(--text-0); border-bottom-color: var(--accent); }

  .libactions { display: flex; gap: var(--sp-2); }

  .libcell { position: relative; display: flex; }
  .libcell > .lib { flex: 1; }
  .del {
    position: absolute; bottom: 6px; right: 6px;
    background: var(--surface-1); border: 1px solid var(--border-0);
    color: var(--text-2); border-radius: var(--r-1);
    width: 20px; height: 20px; line-height: 1; padding: 0;
    cursor: pointer; font-size: 11px; opacity: 0;
    transition: opacity var(--t-fast) var(--ease-out);
  }
  .libcell:hover .del, .del:focus-visible { opacity: 1; }
  .del:hover { color: var(--danger, #e06c75); border-color: var(--danger, #e06c75); }

  /* This one IS modal, unlike the panel: it is irreversible, so it must not be
     dismissible by clicking past it onto something else. */
  .confirm-backdrop {
    position: fixed; inset: 0; background: oklch(0% 0 0 / 0.5);
    z-index: var(--layer-backdrop);
  }
  .confirm {
    position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);
    z-index: var(--layer-modal, 90); width: min(420px, 92vw);
    background: var(--surface-1); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: var(--sp-4);
    display: flex; flex-direction: column; gap: var(--sp-2);
    box-shadow: 0 12px 40px oklch(0% 0 0 / 0.4); outline: none;
  }
  .confirm h3 { margin: 0; font-size: var(--fs-2); }
  .confirm p { margin: 0; font-size: var(--fs-1); }
  button.danger {
    background: var(--danger, #e06c75); color: var(--surface-0); border: 0;
    border-radius: var(--r-1); padding: 7px 12px; cursor: pointer;
    font: inherit; font-size: var(--fs-1);
  }
  button.danger:disabled { opacity: 0.5; cursor: not-allowed; }

  .library {
    display: grid; grid-template-columns: repeat(auto-fill, minmax(108px, 1fr));
    gap: var(--sp-2); padding-bottom: var(--sp-2);
    border-bottom: 1px solid var(--border-0);
  }
  .lib {
    display: flex; flex-direction: column; align-items: flex-start; gap: 2px;
    background: none; border: 1px solid var(--border-0); border-radius: var(--r-1);
    padding: var(--sp-2); cursor: pointer; color: inherit; font: inherit;
    font-size: var(--fs-0); text-align: left; position: relative;
  }
  .lib:hover { background: var(--surface-2); }
  .lib.on { border-color: var(--accent); cursor: default; }
  .lib img, .lib .nothumb {
    width: 100%; aspect-ratio: 3 / 4; object-fit: cover; object-position: top center;
    border-radius: var(--r-1); background: var(--surface-2); display: block;
  }
  .libname { font-weight: 600; color: var(--text-0); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
  .badge {
    position: absolute; top: 6px; right: 6px;
    background: var(--accent); color: var(--surface-0);
    border-radius: var(--r-1); padding: 0 4px; font-size: 10px;
  }
  .warnbadge {
    position: absolute; top: 6px; left: 6px;
    background: var(--warn, #d9a441); color: var(--surface-0);
    border-radius: 50%; width: 14px; height: 14px; display: grid; place-items: center;
    font-size: 10px; font-weight: 700;
  }

  .saved { border: 1px solid var(--border-0); border-radius: var(--r-1); overflow: hidden; }
  .savedhead {
    display: flex; align-items: center; gap: var(--sp-2); width: 100%;
    background: none; border: 0; color: inherit; cursor: pointer;
    padding: var(--sp-2); text-align: left; font: inherit;
  }
  .savedhead:hover { background: var(--surface-2); }
  .savedhead img {
    width: 44px; height: 60px; flex: none; border-radius: var(--r-1);
    object-fit: cover; background: var(--surface-2);
  }
  .savedhead .meta { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; font-size: var(--fs-1); }
  .savedhead .meta .dim { font-size: var(--fs-0); }
  .savedhead .chev { color: var(--text-2); flex: none; }
  .detail {
    display: flex; flex-direction: column; gap: var(--sp-3);
    padding: var(--sp-3); border-top: 1px solid var(--border-0);
  }
  .detail input[type="range"] { accent-color: var(--accent); }

  .addscenes {
    border-top: 1px solid var(--border-0); margin-top: var(--sp-2);
    padding-top: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2);
  }
  .addscenes h3 { margin: 0; font-size: var(--fs-2); }
  .addrow {
    border: 1px solid var(--border-0); border-radius: var(--r-1);
    padding: var(--sp-2) var(--sp-3); display: flex; flex-direction: column; gap: 6px;
  }
  .addhead { display: flex; align-items: center; gap: var(--sp-2); font-size: var(--fs-1); }
  .addhead input { width: auto; }

  .body { padding: var(--sp-4); overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: var(--sp-3); }
  .field { display: flex; flex-direction: column; gap: 6px; font-size: var(--fs-1); }
  .baserow, .keyrow { display: flex; align-items: center; gap: var(--sp-2); }
  .keybox { border: 1px solid var(--border-0); border-radius: var(--r-1); padding: var(--sp-3); gap: var(--sp-2); }
  input, select, textarea {
    background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-1);
    padding: 6px 8px; font: inherit; font-size: var(--fs-1); width: 100%;
  }
  textarea { resize: vertical; line-height: 1.45; }
  .dim { color: var(--text-2); margin: 0; }
  .mono { font-family: var(--font-mono); }
  .warn { color: var(--warn, #d9a441); margin: 0; font-size: var(--fs-1); }
  .err { color: var(--danger, #e06c75); margin: 0; font-size: var(--fs-1); }
  .blocker { color: var(--text-2); margin: 0; font-size: var(--fs-1); font-style: italic; }
  p.warn { color: var(--warn, #d9a441); }

  .slot { border: 1px solid var(--border-0); border-radius: var(--r-1); padding: var(--sp-2) var(--sp-3); }
  .slothead { display: flex; align-items: center; gap: var(--sp-2); font-size: var(--fs-1); }
  .slothead input { width: auto; }
  .conds { display: flex; flex-wrap: wrap; gap: var(--sp-2); margin-top: 6px; }
  .cond { display: flex; align-items: center; gap: 4px; font-size: var(--fs-1); color: var(--text-2); }
  .cond input { width: auto; }

  .summary { font-size: var(--fs-1); padding-top: var(--sp-2); border-top: 1px solid var(--border-0); }
  .runline { font-size: var(--fs-1); }

  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--sp-3); }
  .card { margin: 0; display: flex; flex-direction: column; gap: 4px; }
  .card img, .card .ph {
    width: 100%; aspect-ratio: 3 / 5; object-fit: cover; object-position: top center;
    border-radius: var(--r-1); background: var(--surface-2);
    border: 1px solid var(--border-0);
  }
  .card.failed .ph, .card.skipped .ph { opacity: 0.4; }
  /* The pulse marks the image currently being paid for. Gated on the same
     preference as every other ambient animation in the deck. */
  @media (prefers-reduced-motion: no-preference) {
    .ph.pulse { animation: pulse 1.6s var(--ease-out) infinite; }
  }
  @keyframes pulse { 50% { background: var(--surface-3, var(--surface-2)); opacity: 0.6; } }
  figcaption { font-size: var(--fs-1); display: flex; gap: 6px; flex-wrap: wrap; align-items: baseline; }
  .st { color: var(--text-2); }
  .card.failed .st { color: var(--danger, #e06c75); }

  .actions { display: flex; gap: var(--sp-2); align-items: center; margin-top: auto; padding-top: var(--sp-3); }
  button.primary {
    background: var(--accent); color: var(--surface-0); border: 0;
    border-radius: var(--r-1); padding: 7px 12px; cursor: pointer; font: inherit;
    font-size: var(--fs-1);
  }
  button.primary:disabled { opacity: 0.45; cursor: not-allowed; }
  button.ghost {
    background: none; border: 1px solid var(--border-0); color: var(--text-1);
    border-radius: var(--r-1); padding: 6px 10px; cursor: pointer; font: inherit;
    font-size: var(--fs-1);
  }
  button.ghost:hover { color: var(--text-0); background: var(--surface-2); }
</style>
