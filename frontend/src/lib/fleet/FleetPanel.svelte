<!-- frontend/src/lib/fleet/FleetPanel.svelte — one session's engagement (phase 7a spec §5). All text is DOM text. -->
<script module>
  // Finishes whose jump already played (row key + finish time): the sequence plays once per finish,
  // not again on every refocus or return to Fleet Ops.
  const jumped = new Set();
</script>

<script>
  import { onDestroy } from "svelte";
  import Art from "../raid/Art.svelte";
  import { armourOf, clip } from "../raid/cast.js";
  import { battleOf } from "../raid/battle.js";
  import { lootOf } from "../raid/looks.js";
  import { raid } from "../raid/custom.svelte.js";
  import { labelOf } from "../dashboard/label.js";
  import { shipOf, shipName, encounterOf, actionOf, wingOf, stateOf, veteranOf, fuelOf, spaceOf, fleetMotion } from "./fleet.js";
  import { shipArt, captainArt, captainFallback, craftArt, encounterArt, spaceArt, fxArt, BLEND } from "./art.js";
  let { row, latest = null, events = [], now, hour, animate = true, density = "standard", onopen } = $props();
  const s = $derived(shipOf(row, raid.pack));
  const name = $derived(shipName(row));
  const enc = $derived(encounterOf(row, latest));
  const b = $derived(battleOf(row, events, now));
  const st = $derived(stateOf(b.state));
  const action = $derived(st === "engage" ? actionOf(row.tool) : "idle");
  const wing = $derived(density === "compact" ? { craft: [], more: 0 } : wingOf(row.crew));
  const vet = $derived(veteranOf(row.hero));
  const mo = $derived(fleetMotion(st, vet, animate));
  const fuel = $derived(fuelOf(row));
  const shields = $derived(armourOf(latest));
  const loot = $derived(lootOf(row.hero));
  const label = $derived(labelOf(row));
  const target = $derived(clip(row.targetRel || row.target, 60));
  const canOpen = $derived(!!row.windowID);
  let el;
  let jumping = $state(false), jumpTimer = 0;
  // No cleanup on re-run: row updates every tick and must not cut the sequence short.
  $effect(() => {
    const k = row.key + ":" + row.sinceMs;
    if (st !== "jump" || !animate || jumped.has(k)) return;
    jumped.add(k);
    jumping = true;
    clearTimeout(jumpTimer);
    jumpTimer = setTimeout(() => (jumping = false), 3500);
  });
  onDestroy(() => clearTimeout(jumpTimer));
</script>

<article bind:this={el} class="panel {density} st-{st}" class:anim={animate} class:external={row.external}
  style="--tint:{s.tint}; background-image:url({spaceArt(spaceOf(hour))})"
  data-testid="fleet-panel" data-key={row.key} data-state={st}>
  <header>
    <span class="captain" class:glow={st === "await"} title="Captain {s.captain}" data-testid="fleet-captain">
      <img src={captainArt(s.id, raid.pack)} alt="" draggable="false"
        onerror={(e) => { const b = captainFallback(s.id); if (!e.currentTarget.src.endsWith(b)) e.currentTarget.src = b; }} />
    </span>
    <div class="who">
      <strong data-testid="fleet-name">{name}</strong>
      <span class="cls" data-testid="fleet-class">{s.cls}</span>
      <span class="proj">{label.project}</span>
    </div>
    {#if row.external}<span class="chip ext" data-testid="fleet-external">external</span>{/if}
    {#if row.hero}<span class="chip">Lv {row.hero.level}</span>{/if}
  </header>
  <div class="task">{label.task}</div>

  <div class="stage">
    <div class="ship" class:bob={animate && st === "engage"} class:jumping data-testid="fleet-ship" data-ship={s.id}>
      {#if vet.length}
        <span class="vet" class:pulse={mo.glowPulse} class:trim={vet.includes("trim")} data-testid="fleet-veteran" data-vet={vet.join(" ")}></span>
      {/if}
      <Art src={shipArt(s.id, st === "damaged")} tint={s.tint} />
      {#if vet.includes("escort") && density !== "compact"}
        <span class="escort" class:drift={mo.escortDrift} data-testid="fleet-escort"><Art src={craftArt("wing")} tint={s.tint} quiet /></span>
      {/if}
    </div>
    {#if jumping}
      <span class="vortex out" data-testid="fleet-vortex" data-phase="out"><img src={fxArt("vortex")} alt="" draggable="false" /></span>
      <span class="vortex in" data-testid="fleet-vortex" data-phase="in"><img src={fxArt("vortex")} alt="" draggable="false" /></span>
      <span class="jflash"></span>
    {/if}
    {#if density !== "compact" && st !== "docked"}
      <div class="enc" class:blend={BLEND.has(enc.kind)} class:gone={st === "jump"} data-testid="fleet-encounter" data-kind={enc.kind}>
        <Art src={encounterArt(enc.kind)} tint="#a55" />
        <span class="ename">{enc.name} · Lv {enc.level}</span>
      </div>
    {/if}
    {#if st === "engage"}<span class="fx fx-{action}" data-testid="fleet-action" data-action={action}></span>{/if}
    {#if st === "await"}
      <div class="banner await" data-testid="fleet-banner">
        <span>Awaiting orders, Admiral</span>
        {#if canOpen}<button data-nav onclick={() => onopen(row, el)}>▶ Orders</button>{/if}
      </div>
    {/if}
    {#if st === "jump"}
      <div class="banner jump" data-testid="fleet-cleared">
        <strong>Sector cleared</strong>
        {#if loot}<span data-testid="fleet-loot">{loot.text}</span>{/if}
      </div>
    {/if}
    {#if st === "damaged"}
      <div class="banner hull">
        <strong>Hull breach</strong>
        {#if canOpen}<button data-nav onclick={() => onopen(row, el)}>Repair</button>{/if}
      </div>
    {/if}
  </div>

  {#if wing.craft.length}
    <div class="craft-row">
      {#each wing.craft as c (c.id)}
        <span class="craft" class:away={c.background} data-testid="fleet-craft" data-kind={c.kind} title="{c.type || 'agent'} · {c.description || ''}">
          <img src={craftArt(c.kind)} alt="" draggable="false" />
          <span class="tintlayer" style="-webkit-mask-image:url({craftArt(c.kind)}); mask-image:url({craftArt(c.kind)})"></span>
        </span>
      {/each}
      {#if wing.more > 0}<span class="wing" data-testid="fleet-wing"><img src={craftArt("wing")} alt="" draggable="false" />+{wing.more}</span>{/if}
    </div>
  {/if}

  <footer>
    <span class="bar shields" data-testid="fleet-shields" data-pips={shields} title="Shields (tests)">{"▰".repeat(shields)}{"▱".repeat(3 - shields)}</span>
    {#if fuel != null}<span class="bar fuel" class:low={fuel >= 0.8} data-testid="fleet-fuel" title="Fuel (context used)"><span style="width:{Math.round(fuel * 100)}%"></span></span>{/if}
    {#if st === "engage" && target}<span class="target">{target}</span>{/if}
    <span class="model" data-testid="fleet-model">{row.model || ""}</span>
    {#if canOpen && st !== "await" && st !== "damaged"}<button class="open" data-nav data-testid="fleet-open" onclick={() => onopen(row, el)}>Open</button>{/if}
  </footer>
</article>

<style>
  .panel {
    position: relative; display: flex; flex-direction: column; gap: 6px; padding: var(--sp-3); min-width: 0; overflow: hidden;
    border: 1px solid var(--border-0); border-radius: var(--r-3); background-color: #070b18;
    background-size: cover; background-position: center; color: #eef2fa;
  }
  .panel::before { content: ""; position: absolute; inset: 0; background: linear-gradient(180deg, rgba(4, 6, 14, 0.7), rgba(4, 6, 14, 0.1) 45%, rgba(4, 6, 14, 0.8)); pointer-events: none; }
  .panel > * { position: relative; }
  .panel.external { opacity: 0.8; }
  .st-await { border-color: var(--warn); }
  .st-damaged { border-color: var(--crit); }
  header { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .captain { width: 48px; height: 48px; border-radius: 50%; overflow: hidden; flex: none; border: 2px solid var(--tint); }
  .captain img { width: 100%; height: 100%; object-fit: cover; object-position: top; }
  .captain.glow { box-shadow: 0 0 10px var(--tint); }
  .anim .captain.glow { animation: glow 1.6s ease-in-out infinite; }
  .who { display: flex; flex-direction: column; min-width: 0; line-height: 1.15; }
  .who strong { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .cls, .proj { font-size: var(--fs-0); opacity: 0.8; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chip { margin-left: auto; font-size: var(--fs-0); background: rgba(0, 0, 0, 0.45); border-radius: var(--r-1); padding: 0 5px; }
  .chip.ext { border: 1px dashed rgba(255, 255, 255, 0.5); }
  .task { font-size: var(--fs-1); opacity: 0.9; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
  .stage { position: relative; display: grid; grid-template-columns: 3fr 2fr; align-items: center; height: 170px; }
  .hero .stage { height: 340px; }
  .compact .stage { height: 80px; grid-template-columns: 1fr; }
  .ship { position: relative; height: 100%; display: flex; align-items: center; justify-content: center; filter: drop-shadow(0 0 6px color-mix(in srgb, var(--tint) 50%, transparent)); }
  .ship.bob { animation: bob 3s ease-in-out infinite; }
  .vet { position: absolute; left: 4%; top: 30%; width: 22%; height: 40%; border-radius: 50%; pointer-events: none;
    background: radial-gradient(circle, color-mix(in srgb, var(--tint) 70%, transparent), transparent 70%); opacity: 0.7; }
  .vet.pulse { animation: pulse 2.4s ease-in-out infinite; }
  .vet.trim ~ :global(img) { filter: drop-shadow(0 0 2px #f5c542); }
  .escort { position: absolute; right: 6%; top: 6%; width: 22%; display: flex; opacity: 0.9; }
  .escort.drift { animation: drift 5s ease-in-out infinite; }
  .enc { position: relative; height: 100%; display: flex; align-items: center; justify-content: center; }
  /* blend + mask the image only, so the encounter name stays readable */
  .enc.blend :global(img) { mix-blend-mode: screen; -webkit-mask-image: radial-gradient(circle, #000 55%, transparent 75%); mask-image: radial-gradient(circle, #000 55%, transparent 75%); }
  .enc.gone { opacity: 0; transition: opacity 1.2s ease-out; }
  .ename { position: absolute; top: 4px; left: 0; right: 0; text-align: center; font-size: var(--fs-0); text-shadow: 0 1px 2px #000; }
  .fx { position: absolute; left: 50%; top: 45%; width: 22%; height: 6px; pointer-events: none; opacity: 0.85; }
  .fx-beam { height: 3px; background: linear-gradient(90deg, var(--tint), #fff); box-shadow: 0 0 8px var(--tint); }
  .fx-sweep { width: 40px; height: 40px; left: 46%; top: 35%; border-radius: 50%; border: 2px solid var(--tint); }
  .fx-salvo { background: radial-gradient(circle, #ffd27a 2px, transparent 3px) 0 0 / 12px 6px repeat-x; }
  .fx-scan { width: 60px; height: 60px; left: 44%; top: 28%; border-radius: 50%; border: 1px dashed var(--tint); }
  .fx-launch { height: 2px; background: repeating-linear-gradient(90deg, var(--tint) 0 6px, transparent 6px 12px); }
  .anim .fx-beam, .anim .fx-salvo, .anim .fx-launch { animation: fire 900ms linear infinite; }
  .anim .fx-sweep, .anim .fx-scan { animation: ping 1.4s ease-out infinite; }
  .st-damaged .ship { filter: drop-shadow(0 0 8px #ff4d3d); }
  .anim.st-damaged .ship { animation: flicker 1.4s steps(1, end) infinite; }
  @keyframes flicker { 0%, 100% { opacity: 1; } 8% { opacity: 0.55; } 12% { opacity: 1; } 52% { opacity: 0.7; } 55% { opacity: 1; } }
  .st-docked .ship { opacity: 0.72; }
  .banner { position: absolute; z-index: 2; left: 0; right: 0; bottom: 8%; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 6px; background: rgba(0, 0, 0, 0.62); font-size: var(--fs-1); }
  .banner.await { border-top: 1px solid var(--warn); }
  .banner.jump strong { letter-spacing: 0.12em; text-transform: uppercase; }
  .banner button, .open { background: rgba(0, 0, 0, 0.5); color: inherit; border: 1px solid #cfd8ea; border-radius: var(--r-2); padding: 2px 10px; cursor: pointer; font-size: var(--fs-1); }
  .craft-row { display: flex; align-items: center; gap: 6px; min-height: 30px; }
  .craft { position: relative; width: 52px; height: 30px; display: flex; }
  .craft img { width: 100%; height: 100%; object-fit: contain; }
  .craft .tintlayer { position: absolute; inset: 0; background: var(--tint); mix-blend-mode: color; -webkit-mask-size: contain; mask-size: contain;
    -webkit-mask-repeat: no-repeat; mask-repeat: no-repeat; -webkit-mask-position: center; mask-position: center; }
  .craft.away { opacity: 0.4; }
  .wing { font-size: var(--fs-0); opacity: 0.85; display: inline-flex; align-items: center; gap: 2px; }
  .wing img { height: 18px; width: auto; }
  .model { font-size: var(--fs-0); opacity: 0.7; font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  footer { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: var(--fs-0); }
  .bar.shields { color: #9cd3ff; letter-spacing: 1px; }
  .bar.fuel { width: 70px; height: 5px; background: rgba(255, 255, 255, 0.15); border-radius: 3px; overflow: hidden; display: inline-block; }
  .bar.fuel span { display: block; height: 100%; background: linear-gradient(90deg, #5fd0ff, #b2f0ff); }
  .bar.fuel.low span { background: linear-gradient(90deg, #ffb15b, #ff6b5b); }
  .target { font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; max-width: 50%; }
  .open { margin-left: auto; }
  @keyframes bob { 50% { transform: translateY(-4px); } }
  @keyframes pulse { 50% { opacity: 0.35; } }
  @keyframes drift { 50% { transform: translate(-6px, 4px); } }
  @keyframes glow { 50% { opacity: 0.6; } }
  @keyframes fire { from { transform: translateX(-10%); opacity: 0.9; } to { transform: translateX(30%); opacity: 0.2; } }
  @keyframes ping { from { transform: scale(0.5); opacity: 0.9; } to { transform: scale(1.4); opacity: 0; } }
  /* Babylon 5 jump point: out through one vortex, back through another. Finite; ends as still mode. */
  .vortex { position: absolute; top: 50%; height: 88%; aspect-ratio: 1; translate: -50% -50%; opacity: 0; transform: scale(0); pointer-events: none; mix-blend-mode: screen; z-index: 0; }
  .vortex img { width: 100%; height: 100%; border-radius: 50%; animation: vspin 3.4s linear 1 forwards;
    -webkit-mask-image: radial-gradient(circle, #000 55%, transparent 70%); mask-image: radial-gradient(circle, #000 55%, transparent 70%); }
  .vortex.out { left: 72%; animation: vout 3.4s ease-in-out 1 forwards; }
  .vortex.in { left: 30%; animation: vin 3.4s ease-in-out 1 forwards; }
  .jflash { position: absolute; left: 72%; top: 50%; width: 10px; height: 10px; translate: -50% -50%; border-radius: 50%; background: #dff4ff; box-shadow: 0 0 40px 20px #9fd8ff; opacity: 0; pointer-events: none; animation: jflash 3.4s ease-out 1 forwards; }
  .ship.jumping { z-index: 1; animation: jumpship 3.4s ease-in-out 1 forwards; }
  @keyframes vspin { to { transform: rotate(-540deg); } }
  @keyframes vout { 0% { opacity: 0; transform: scale(0); } 16%, 40% { opacity: 1; transform: scale(1); } 48%, 100% { opacity: 0; transform: scale(0.02); } }
  @keyframes vin { 0%, 55% { opacity: 0; transform: scale(0); } 66%, 84% { opacity: 1; transform: scale(1.1); } 96%, 100% { opacity: 0; transform: scale(0); } }
  @keyframes jflash { 0%, 44% { opacity: 0; transform: scale(0.2); } 48% { opacity: 1; transform: scale(3); } 56%, 100% { opacity: 0; transform: scale(5); } }
  /* the ship column is 60 % of the stage, so +70 % of its width lands on the out-vortex at 72 % */
  @keyframes jumpship {
    0%, 14% { transform: none; opacity: 1; }
    30% { transform: translateX(50%) scale(0.55); opacity: 1; filter: brightness(1.2); }
    38% { transform: translateX(70%) scale(0.12); opacity: 0.8; filter: brightness(1.8); }
    40%, 64% { transform: translateX(70%) scale(0.04); opacity: 0; }
    65% { transform: scale(0.08); opacity: 0; }
    80% { transform: scale(1.05); opacity: 1; filter: brightness(1.3); }
    100% { transform: none; opacity: 1; }
  }
</style>
