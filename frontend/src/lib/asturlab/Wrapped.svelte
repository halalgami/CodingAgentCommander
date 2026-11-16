<script>
  // Commander Wrapped (spec §7): year-in-review cards drawn as one SVG so the
  // PNG export is exactly what is on screen. No model call; nothing uploaded.
  import { onMount } from "svelte";
  import { Wrapped, SaveWrappedPNG } from "../../../wailsjs/go/main/App.js";
  import { cardsOf } from "./wrapped.js";

  let { onclose } = $props();
  let year = $state(new Date().getFullYear());
  let view = $state(null);
  let saved = $state("");
  let svgEl = $state(null);

  async function load(y) { year = y; saved = ""; try { view = await Wrapped(y); } catch { view = { year: y, empty: true, years: [y] }; } }
  onMount(() => load(year));

  const cards = $derived(cardsOf(view));
  const W = 1080, H = 1350, PAD = 48, GAP = 24;
  // Title card spans the top; the rest fill a 2-column grid.
  const boxes = $derived(cards.map((c, i) => {
    if (i === 0) return { c, x: PAD, y: PAD + 90, w: W - 2 * PAD, h: 240 };
    const k = i - 1, col = k % 2, row = Math.floor(k / 2);
    const w = (W - 2 * PAD - GAP) / 2, h = 260;
    return { c, x: PAD + col * (w + GAP), y: PAD + 90 + 240 + GAP + row * (h + GAP), w, h };
  }));

  async function save() {
    if (!svgEl || view?.empty) return;
    const xml = new XMLSerializer().serializeToString(svgEl);
    const img = new Image();
    img.src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(xml);
    await img.decode();
    const cv = document.createElement("canvas");
    cv.width = W; cv.height = H;
    cv.getContext("2d").drawImage(img, 0, 0, W, H);
    try { saved = (await SaveWrappedPNG(cv.toDataURL("image/png"))) || ""; } catch (e) { saved = "Could not save: " + e; }
  }
  function onKey(e) { if (e.key === "Escape") onclose?.(); }
</script>

<svelte:window onkeydown={onKey} />
<div class="wrap" data-testid="ast-wrapped" role="dialog" aria-label="Commander Wrapped">
  <div class="bar">
    <strong>Commander Wrapped</strong>
    <select aria-label="Year" data-testid="ast-wrapped-year" value={year} onchange={(e) => load(+e.currentTarget.value)}>
      {#each view?.years?.length ? view.years : [year] as y}<option value={y}>{y}</option>{/each}
    </select>
    <button data-testid="ast-wrapped-save" disabled={!view || view.empty} onclick={save}>Save PNG</button>
    {#if saved}<span class="dim" data-testid="ast-wrapped-saved">{saved}</span>{/if}
    <span class="dim">Made on this Mac; nothing is uploaded.</span>
    <button class="close" aria-label="Close" data-testid="ast-wrapped-close" onclick={() => onclose?.()}>✕</button>
  </div>
  <svg bind:this={svgEl} xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width={W} height={H} class="art" role="img" aria-label="Wrapped {year}">
    <rect width={W} height={H} fill="#0e1726" />
    <text x={PAD} y={PAD + 50} fill="#86b6ef" font-family="Helvetica, Arial, sans-serif" font-size="44" font-weight="700">Commander Wrapped · {year}</text>
    {#each boxes as b (b.c.id)}
      <g data-testid="ast-wrapped-card" data-card={b.c.id}>
        <rect x={b.x} y={b.y} width={b.w} height={b.h} rx="20" fill="#16243a" stroke="#256abf" stroke-width="2" />
        <text x={b.x + 28} y={b.y + 52} fill="#9fb3cc" font-family="Helvetica, Arial, sans-serif" font-size="28">{b.c.label}</text>
        {#if b.c.list}
          {#each b.c.list.slice(0, 5) as item, i}
            <text x={b.x + 28} y={b.y + 100 + i * 34} fill="#ffffff" font-family="Helvetica, Arial, sans-serif" font-size="28">{i + 1}. {item.length > 24 ? item.slice(0, 23) + "…" : item}</text>
          {/each}
        {:else}
          <text x={b.x + 28} y={b.y + (b.c.id === "title" ? 150 : 140)} fill="#ffffff" font-family="Helvetica, Arial, sans-serif" font-size={b.c.id === "title" ? 96 : 64} font-weight="700">{b.c.big}</text>
          {#if b.c.sub}<text x={b.x + 28} y={b.y + b.h - 36} fill="#9fb3cc" font-family="Helvetica, Arial, sans-serif" font-size="26">{b.c.sub}</text>{/if}
        {/if}
      </g>
    {/each}
  </svg>
</div>

<style>
  .wrap { position: absolute; inset: 0; z-index: 20; background: var(--surface-0); display: flex; flex-direction: column; gap: var(--sp-2); padding: var(--sp-3); }
  .bar { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
  .dim { color: var(--text-2); font-size: var(--fs-0); }
  .close { margin-left: auto; }
  .art { flex: 1; min-height: 0; width: 100%; height: 100%; }
</style>
