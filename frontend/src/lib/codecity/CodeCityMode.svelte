<script>
  import { onMount, onDestroy, untrack } from "svelte";
  import { layoutCode } from "./layout.js";
  import { heatOf, cranesFor } from "./look.js";
  import { paintCodeGround, paintCodeStatic, paintHeat, paintCodeLive, hitLot, lotScreen } from "./painter.js";
    import { SPRITES } from "../city/manifest.js";
  import { loadSprites, releaseSprites } from "../city/sprites.js";
  import { fit, zoomAt, pan as panCam, worldToView, wheelFactor } from "../city/camera.js";
  import { toScreen } from "../city/iso.js";
  import { activityText } from "../activity.js";
  import { ScanProject } from "../../../wailsjs/go/main/App.js";

  let { zoom, rows, now, animate = true, onopen, onback } = $props();

  let host = $state(null), groundCv = $state(null), staticCv = $state(null), heatCv = $state(null), blurCv = $state(null), liveCv = $state(null), overlay = $state(null);
  let vw = $state(800), vh = $state(500), cam = $state(null), hoverKey = $state(null);
  let heatOn = $state(false), heatWin = $state("30d");
  let sprites = new Map(), spritesReady = $state(0), destroyed = false, raf = 0, sized = $state(0);

  const view = $derived(zoom.view || { files: [], slabs: [], folders: [] });
  const layout = $derived(layoutCode(view));
  const boundsKey = $derived(JSON.stringify((() => {
    const s = layout.side, a = toScreen(0, 0), b = toScreen(s, s), c = toScreen(s, 0), d = toScreen(0, s);
    return { minX: d.sx, maxX: c.sx, minY: a.sy, maxY: b.sy };
  })()));
  const camera = $derived(cam || fit(JSON.parse(boundsKey), vw, vh));
  // Heat changes by the minute: a per-second now must not repaint it.
  const heatMinute = $derived(Math.floor(now / 60000));
  const heat = $derived(heatOf(view.files, heatWin, heatMinute * 60000));
  // The hover card is looked up by key in the current view, so a refreshed
  // CodeView updates its counts without the mouse moving.
  const hover = $derived(!hoverKey ? null : hoverKey.startsWith("slab:")
    ? slabHover(view.slabs.find((s) => s.folder === hoverKey.slice(5)))
    : view.files.find((f) => f.path === hoverKey) || null);
  const cranes = $derived(cranesFor(rows, zoom.key));
  const signs = $derived(layout.lots.filter((l) => l.kind === "file" && cranes.byFile.has(l.ref.path)));

  function size(cv) {
    const dpr = window.devicePixelRatio || 1;
    cv.width = Math.round(vw * dpr); cv.height = Math.round(vh * dpr);
    const ctx = cv.getContext("2d"); ctx?.setTransform(dpr, 0, 0, dpr, 0, 0); return ctx;
  }
  $effect(() => {
    void vw; void vh;
    if (!groundCv || !staticCv || !heatCv || !blurCv || !liveCv) return;
    [groundCv, staticCv, heatCv, blurCv, liveCv].forEach(size);
    untrack(() => sized++);
  });
  $effect(() => {
    void spritesReady; void sized;
    if (!sized) return;
    const l = layout, c = camera;
    untrack(() => {
      paintCodeGround(groundCv.getContext("2d"), l, c, vw, vh);
      paintCodeStatic(staticCv.getContext("2d"), l, sprites, c, vw, vh);
      const b = blurCv.getContext("2d");
      b.clearRect(0, 0, vw, vh);
      b.filter = "blur(3px)";
      b.drawImage(groundCv, 0, 0, vw, vh);
      b.drawImage(staticCv, 0, 0, vw, vh);
      b.filter = "none";
      window.__codeStatic = (window.__codeStatic || 0) + 1;
      drawLive(performance.now());
    });
  });
  $effect(() => {
    void sized;
    if (!sized) return;
    const l = layout, c = camera, h = heatOn ? heat : new Map();
    untrack(() => { paintHeat(heatCv.getContext("2d"), l, h, c, vw, vh); window.__codeHeat = (window.__codeHeat || 0) + 1; });
  });
  function drawLive(t) {
    if (!liveCv || !sized) return;
    paintCodeLive(liveCv.getContext("2d"), layout, cranes, sprites, camera, vw, vh, t, now);
    window.__codeFrames = (window.__codeFrames || 0) + 1;
  }
  $effect(() => {
    // Only cranes and workers move; with none on screen there is nothing to animate.
    if (!animate || !cranes.shown) return;
    let last = 0;
    const loop = (t) => { raf = requestAnimationFrame(loop); if (t - last < 33) return; last = t; drawLive(t); };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  });
  $effect(() => { void now; void cranes; if (!animate) untrack(() => drawLive(performance.now())); });

  onMount(() => {
    const ro = new ResizeObserver(() => { vw = host.clientWidth; vh = host.clientHeight; });
    ro.observe(host);
    host?.focus({ preventScroll: true });
    loadSprites(SPRITES).then((m) => { if (destroyed) releaseSprites(m); else { sprites = m; spritesReady++; } });
    window.__codeHover = (p) => (hoverKey = p);
    return () => ro.disconnect();
  });
  onDestroy(() => { destroyed = true; cancelAnimationFrame(raf); releaseSprites(sprites); delete window.__codeHover; });

  function slabHover(s) { return s ? { slab: true, path: s.folder || "(root)", files: s.files, code: s.code } : null; }
  let drag = null;
  function move(e) {
    const r = host.getBoundingClientRect();
    if (drag) { cam = panCam(camera, e.clientX - drag.x, e.clientY - drag.y); drag = { x: e.clientX, y: e.clientY }; return; }
    const lot = hitLot(layout, camera, vw, vh, e.clientX - r.left, e.clientY - r.top);
    hoverKey = lot?.kind === "slab" ? "slab:" + lot.ref.folder : lot?.ref?.path || null;
  }
  // Scroll and pinch zoom at the cursor, drag pans — as in Al Modon.
  function wheel(e) {
    e.preventDefault();
    const r = host.getBoundingClientRect();
    cam = zoomAt(camera, wheelFactor(e), e.clientX - r.left, e.clientY - r.top, vw, vh);
  }
  function keys(e) {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onback(); return; }
    if (e.key === "h" || e.key === "H") { heatOn = !heatOn; return; }
    const m = { ArrowLeft: [40, 0], ArrowRight: [-40, 0], ArrowUp: [0, 40], ArrowDown: [0, -40] }[e.key];
    if (m) { cam = panCam(camera, m[0], m[1]); e.preventDefault(); e.stopPropagation(); }
    else if (e.key === "+" || e.key === "=") cam = zoomAt(camera, 1.25, vw / 2, vh / 2, vw, vh);
    else if (e.key === "-") cam = zoomAt(camera, 0.8, vw / 2, vh / 2, vw, vh);
    else if (e.key === "0") cam = null;
  }
  const at = (lot) => { const p = lotScreen(lot); return worldToView(camera, p.sx, p.sy, vw, vh); };
  const folderAt = (f) => { const p = toScreen(f.x + f.w / 2, f.y + f.h); return worldToView(camera, p.sx, p.sy, vw, vh); };
  const folderWide = (f) => f.w * 32 * camera.scale >= 60;
  const ago = (ms) => (ms ? new Date(ms).toLocaleDateString() : "never");
</script>

<!-- Keys live on the host: a click on a canvas focuses it (tabindex -1), so they keep working. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="code" class:enter={animate} data-testid="dash-codecity" role="application" aria-label="CodeCity" tabindex="-1" bind:this={host} onkeydown={keys}
  style:transform-origin={zoom.origin ? `${zoom.origin.x}px ${zoom.origin.y}px` : null}>
  <canvas class="layer" bind:this={groundCv} style="width:{vw}px;height:{vh}px"></canvas>
  <canvas class="layer" bind:this={heatCv} style="width:{vw}px;height:{vh}px" aria-hidden="true"></canvas>
  <canvas class="layer bldg" class:faded={heatOn} data-testid="code-buildings" bind:this={staticCv} style="width:{vw}px;height:{vh}px"></canvas>
  <canvas class="layer blur" bind:this={blurCv} style="width:{vw}px;height:{vh}px" aria-hidden="true"></canvas>
  <canvas class="layer" bind:this={liveCv} style="width:{vw}px;height:{vh}px"
    onpointerdown={(e) => (drag = { x: e.clientX, y: e.clientY })} onpointermove={move} onpointerup={() => (drag = null)} onpointerleave={() => (drag = null)} onwheel={wheel}></canvas>

  <div class="overlay" bind:this={overlay} role="region" aria-label="CodeCity details">
    <nav class="crumb" data-testid="code-breadcrumb">
      <button data-testid="code-breadcrumb-city" onclick={onback}>Al Modon</button> › <span>{zoom.label}</span>
    </nav>
    {#if !zoom.view}
      <div class="note" data-testid="code-loading">Loading…</div>
    {:else if view.locked}
      <div class="note">Turn on project history to zoom in.</div>
    {:else if view.noMetrics}
      <div class="note">Not scanned yet. {#if !view.missing}<button data-testid="code-scan" onclick={() => ScanProject(zoom.key).catch(() => {})}>Scan</button>{/if}</div>
    {/if}
    {#if view.missing}<div class="note banner" data-testid="code-missing">Folder missing — showing the last scan.</div>{/if}
    {#each layout.folders as f (f.path)}
      {#if f.depth > 0 && folderWide(f)}
        {@const p = folderAt(f)}
        <span class="folder" style="left:{p.vx}px;top:{p.vy}px">{f.label}{f.hidden ? ` +${f.hidden}` : ""}</span>
      {/if}
    {/each}
    {#each signs as lot (lot.ref.path)}
      {@const p = at(lot)}
      {#each cranes.byFile.get(lot.ref.path).slice(0, 1) as c (c.row.key)}
        <div class="sign" data-testid="code-sign" style="left:{p.vx}px;top:{p.vy - 60 * camera.scale}px">
          <span class="t">{c.row.title}</span>
          {#if c.row.activity === "permission"}<span class="beacon" data-testid="code-beacon">Needs you</span>{/if}
          <span class="a">{c.agent ? `${c.agent.type || "agent"} · ${c.agent.tool}` : activityText(c.row)}</span>
          {#if c.row.windowID}<button onclick={(e) => onopen(c.row, e.currentTarget)}>{c.row.activity === "permission" ? "Review →" : "Open"}</button>{/if}
        </div>
      {/each}
    {/each}
    {#if cranes.more}<div class="more">+{cranes.more} more cranes</div>{/if}
    {#if hover}
      <div class="hover" data-testid="code-hover">
        <div class="t">{hover.path}</div>
        {#if hover.slab}
        <div class="m">{hover.files} more files · {hover.code} lines (not drawn one by one)</div>
        {:else}
        <div class="m">{hover.code} lines · complexity {hover.complexity}{hover.flagged ? " · hotspot" : ""}{hover.ghost ? " · deleted" : ""}{hover.new ? " · new" : ""}</div>
        <div class="m">Claude: {hover.touch.edits} edits · {hover.touch.writes} writes · {hover.touch.reads} reads · last {ago(hover.touch.lastAt)}</div>
        {/if}
      </div>
    {/if}
    <div class="tools">
      <button data-testid="code-heat" aria-pressed={heatOn} onclick={() => (heatOn = !heatOn)}>Heat (H)</button>
      {#if heatOn}
        <span class="legend" data-testid="code-heat-legend">cool → hot ·
          {#each ["7d", "30d", "all"] as w (w)}<button class:on={heatWin === w} onclick={() => (heatWin = w)}>{w}</button>{/each}
        </span>
      {/if}
      <span class="zoom" data-testid="code-zoom" data-scale={camera.scale.toFixed(3)}>
        <button data-testid="code-zoom-out" title="Zoom out (−)" onclick={() => (cam = zoomAt(camera, 0.8, vw / 2, vh / 2, vw, vh))}>−</button>
        <button data-testid="code-fit" title="Fit (0)" onclick={() => (cam = null)}>Fit</button>
        <button data-testid="code-zoom-in" title="Zoom in (+)" onclick={() => (cam = zoomAt(camera, 1.25, vw / 2, vh / 2, vw, vh))}>+</button>
      </span>
    </div>
  </div>
</div>

<style>
  .code { position: absolute; inset: 0; overflow: hidden; outline: none; background: linear-gradient(#e9eef2, #dfe6d6); }
  /* Zoom-in from the district (spec §5); still mode skips it. */
  .code.enter { animation: zoomin 320ms ease-out; }
  @keyframes zoomin { from { transform: scale(0.35); opacity: 0.2; } to { transform: none; opacity: 1; } }
  .layer { position: absolute; inset: 0; }
  /* Heat on: buildings fade so the heat-tinted ground reads as a map. */
  .bldg { transition: opacity 160ms; }
  .bldg.faded { opacity: 0.35; }
  .blur { pointer-events: none; -webkit-mask-image: linear-gradient(#000 0%, transparent 28%, transparent 72%, #000 100%); mask-image: linear-gradient(#000 0%, transparent 28%, transparent 72%, #000 100%); }
  .overlay { position: absolute; inset: 0; pointer-events: none; outline: none; }
  .overlay > * { pointer-events: auto; }
  .crumb { position: absolute; left: 16px; top: 12px; background: var(--surface-1); color: var(--text-1); border-radius: 6px; padding: 3px 10px; font-size: var(--fs-0); }
  .crumb button { background: none; border: 0; color: var(--accent, var(--text-0)); cursor: pointer; padding: 0; font: inherit; }
  .note { position: absolute; left: 50%; top: 12px; transform: translateX(-50%); background: var(--surface-1); color: var(--text-1); border-radius: 6px; padding: 4px 10px; font-size: var(--fs-0); }
  .banner { top: 44px; color: var(--warn); }
  .folder { position: absolute; transform: translate(-50%, 0); background: rgba(255,255,255,0.8); color: #333; border-radius: 4px; padding: 0 6px; font-size: var(--fs-0); white-space: nowrap; pointer-events: none; }
  .sign { position: absolute; transform: translate(-50%, -100%); background: var(--surface-1); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 6px; padding: 3px 8px; display: flex; flex-direction: column; gap: 2px; max-width: 260px; font-size: var(--fs-0); }
  .sign .t { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sign .beacon { color: var(--warn); font-weight: 600; }
  .zoom { display: flex; gap: 4px; }
  .sign .a { color: var(--text-1); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sign button, .tools button { align-self: flex-start; background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 4px; font-size: var(--fs-0); cursor: pointer; }
  .tools { position: absolute; right: 16px; bottom: 16px; display: flex; gap: 6px; align-items: center; }
  .legend { background: linear-gradient(90deg, #3b6fb6, #e0452b); color: #fff; border-radius: 4px; padding: 2px 6px; font-size: var(--fs-0); }
  .legend button.on { font-weight: 700; }
  .more { position: absolute; left: 16px; top: 44px; color: var(--text-1); font-size: var(--fs-0); }
  .hover { position: absolute; left: 16px; bottom: 16px; background: var(--surface-1); color: var(--text-0); border: 1px solid var(--border-1); border-radius: 6px; padding: 6px 10px; max-width: 420px; }
  .hover .t { font-family: var(--font-mono); word-break: break-all; }
  .hover .m { color: var(--text-2); font-size: var(--fs-0); }
</style>
