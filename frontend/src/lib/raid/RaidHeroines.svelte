<script>
  // frontend/src/lib/raid/RaidHeroines.svelte
  // Settings → Raid heroines (heroine packs spec §6): replace any heroine image.
  import { onMount } from "svelte";
  import { raid, setRaidPack } from "./custom.svelte.js";
  import { HEROINES } from "./cast.js";
  import { POSES, bundledArt } from "./art.js";
  import { captainArt } from "../fleet/art.js";
  import { toast } from "../stores.svelte.js";
  import * as G from "../../../wailsjs/go/main/App.js";

  const EXTRA = (id) => [
    ...["v1", "v2", "legend"].flatMap((v) => POSES.map((p) => `${id}-${v}-${p}`)),
    `gear-familiar-${id}`,
    `captain-${id}`,
  ];
  let packs = $state([]);
  let open = $state({});
  let arming = $state(""); // a pack's dir after the first Delete click; the second click deletes

  async function run(fn) {
    try {
      const v = await fn();
      if (v && typeof v === "object") setRaidPack(v);
      await refreshPacks();
    } catch (e) { toast(String(e?.message || e), "error"); }
  }
  async function refreshPacks() { try { packs = (await G.ListRaidPacks()) || []; } catch { packs = []; } }
  async function exportKit() {
    try { const p = await G.ExportRaidStarter(); if (p) toast("Starter kit saved", "info"); }
    catch (e) { toast(String(e?.message || e), "error"); }
  }
  onMount(refreshPacks);

  const src = (stem) => raid.pack.files[stem] || (stem.startsWith("captain-") ? captainArt(stem.slice(8)) : bundledArt(stem));
  const custom = (stem) => !!raid.pack.files[stem];
</script>

<div class="raid-heroines" data-testid="raid-heroines">
  <div class="packline">
    <span data-testid="raid-pack-name">{raid.pack.dir ? raid.pack.name : "Built-in heroines"}</span>
    {#if packs.length}
      <select data-testid="raid-pack-select" value={raid.pack.dir}
        onchange={(e) => run(() => (e.target.value ? G.SelectRaidPack(e.target.value) : G.ClearRaidPack()))}>
        <option value="">Built-in heroines</option>
        {#each packs as p (p.dir)}<option value={p.dir}>{p.name} ({p.count})</option>{/each}
      </select>
    {/if}
  </div>
  <div class="actions">
    <button data-testid="raid-import" onclick={() => run(G.ImportRaidPack)}>Import zip…</button>
    <button data-testid="raid-folder" onclick={() => run(G.ChooseRaidFolder)}>Choose folder…</button>
    <button data-testid="raid-export" onclick={exportKit}>Export starter kit…</button>
    {#if raid.pack.dir}<button data-testid="raid-clear" onclick={() => run(G.ClearRaidPack)}>Use built-in</button>{/if}
  </div>
  {#if packs.some((p) => p.dir !== raid.pack.dir)}
    <ul class="packs">
      {#each packs.filter((p) => p.dir !== raid.pack.dir) as p (p.dir)}
        <li>{p.name} <span class="dim">({p.count})</span>
          <button data-testid="raid-pack-delete" data-dir={p.dir} aria-label="Delete {p.name}"
            onclick={() => (arming === p.dir ? run(() => G.DeleteRaidPack(p.dir)).then(() => (arming = "")) : (arming = p.dir))}>{arming === p.dir ? "Delete for good?" : "Delete"}</button></li>
      {/each}
    </ul>
  {/if}
  <p class="dim">Click or drop an image on a cell. A heroine is fully yours once her stance is replaced.</p>

  {#each HEROINES as h (h.id)}
    {@const o = raid.pack.heroines[h.id] || {}}
    <div class="row" data-testid="raid-row" data-id={h.id}>
      <div class="who">
        <input data-testid="raid-heroine-name" placeholder={h.name} value={o.name || ""} maxlength="24"
          onchange={(e) => run(() => G.SetRaidHeroine(h.id, e.target.value, o.tint || ""))} />
        <input type="color" data-testid="raid-heroine-tint" value={o.tint || h.tint}
          onchange={(e) => run(() => G.SetRaidHeroine(h.id, o.name || "", e.target.value))} />
      </div>
      <div class="cells">
        {#each [...POSES.map((p) => `${h.id}-${p}`), ...(open[h.id] ? EXTRA(h.id) : [])] as stem (stem)}
          <div class="cell" data-testid="raid-cell" data-stem={stem} data-raid-stem={stem} data-custom={custom(stem)}
            style="--wails-drop-target: drop" title={stem}>
            <button class="pick" onclick={() => run(() => G.ReplaceRaidImage(stem))} aria-label="Replace {stem}">
              <img src={src(stem)} alt="" />
            </button>
            {#if custom(stem)}
              <button class="reset" data-testid="raid-cell-reset" aria-label="Reset {stem}" onclick={() => run(() => G.ResetRaidImage(stem))}>↺</button>
            {/if}
          </div>
        {/each}
      </div>
      <button class="more" data-testid="raid-more" onclick={() => (open[h.id] = !open[h.id])}>{open[h.id] ? "Fewer images" : "More images"}</button>
    </div>
  {/each}

  {#if raid.pack.warnings.length}
    <ul class="warnings" data-testid="raid-pack-warnings">{#each raid.pack.warnings as w}<li>{w}</li>{/each}</ul>
  {/if}
</div>

<style>
  .raid-heroines { display: grid; gap: 8px; }
  .packline, .actions, .who { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
  .row { display: grid; gap: 4px; padding: 6px 0; border-top: 1px solid var(--line, #3335); }
  .who input:not([type="color"]) { flex: 1; min-width: 0; }
  .cells { display: flex; flex-wrap: wrap; gap: 4px; }
  .cell { position: relative; width: 44px; height: 56px; border-radius: 6px; background: var(--panel-2, #0002); }
  .cell[data-custom="true"] { outline: 2px solid var(--accent, #6af); }
  .cell :global(.wails-drop-target-active) , .cell:global(.wails-drop-target-active) { outline: 2px dashed var(--accent, #6af); }
  .pick { all: unset; cursor: pointer; display: block; width: 100%; height: 100%; }
  .pick img { width: 100%; height: 100%; object-fit: contain; }
  .reset { position: absolute; top: -6px; right: -6px; font-size: 10px; padding: 0 4px; border-radius: 8px; }
  .more { justify-self: start; font-size: 11px; }
  .packs { list-style: none; margin: 0; padding: 0; font-size: 12px; display: grid; gap: 2px; }
  .packs button { font-size: 11px; margin-left: 6px; }
  .dim { opacity: .7; font-size: 12px; margin: 0; }
  .warnings { font-size: 12px; color: var(--warn, #e9a23b); margin: 0; padding-left: 16px; }
</style>
