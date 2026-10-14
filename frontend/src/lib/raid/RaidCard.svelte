<!-- frontend/src/lib/raid/RaidCard.svelte — one session's fight (spec §4–5). All text is DOM text. -->
<script>
  import { untrack } from "svelte";
  import Art from "./Art.svelte";
  import { heroineOf, bossOf, BOSS_NAMES, bossLevel, tierOf, armourOf, moveOf, partyOf, clip } from "./cast.js";
  import { battleOf, lastHitAt } from "./battle.js";
  import { heroineArt, bossArt, arenaArt, arenaOf, allyArt, slimeArt, innArt, chestArt } from "./art.js";
  import { labelOf } from "../dashboard/label.js";
  import Radar from "./Radar.svelte";
  import { lookOf, ascensionOf, gearOf, motionBudget, wingsOf, radarOf, badgesOf, pouchOf, lootOf } from "./looks.js";
  import { gearArt, haloArt } from "./art.js";
  import { raid } from "./custom.svelte.js";
  let { row, latest = null, events = [], now, hour, animate = true, density = "standard", progress = null, quotaPct = null, onopen } = $props();
  const h = $derived(heroineOf(row, raid.pack));
  const kind = $derived(bossOf(row));
  const level = $derived(bossLevel(latest));
  const tier = $derived(tierOf(latest));
  const armour = $derived(armourOf(latest));
  const b = $derived(battleOf(row, events, now));
  // The damage number shows for any hit that lands while Raid is open (whatever
  // its transcript timestamp), and for the hit already there on open only when
  // it is under 5 s old — so reopening Raid never replays an old one.
  const hitAt = $derived(lastHitAt(events));
  const openedWith = untrack(() => lastHitAt(events));
  // When this card first saw its latest hit: a hit shows for 5 s from then, so a
  // card returning to attack later never replays an old one.
  let seenAt = $state(untrack(() => (openedWith ? openedWith : 0)));
  $effect(() => {
    const h = hitAt;
    untrack(() => { if (h && h !== openedWith) seenAt = Date.now(); });
  });
  const freshHit = $derived(!!hitAt && now - seenAt < 5000);
  const party = $derived(partyOf(row.crew));
  const label = $derived(labelOf(row));
  const target = $derived(clip(row.targetRel || row.target, 80));
  const canOpen = $derived(!!row.windowID);
  const minions = $derived(Math.min(6, party.allies.filter((a) => a.state === "working" || a.state === "tool").length));
  const look = $derived(lookOf(row));
  const asc = $derived(ascensionOf(row.hero));
  const gear = $derived(density === "compact" ? [] : gearOf(row.hero));
  const mb = $derived(motionBudget(b.state, asc, animate));
  const radar = $derived(density === "compact" ? null : radarOf(row.hero));
  const badges = $derived(badgesOf(row, progress));
  const pouch = $derived(pouchOf(quotaPct));
  const loot = $derived(lootOf(row.hero));
  let el;
</script>

<article bind:this={el} class="card {density} st-{b.state}" class:external={row.external} class:enraged={b.enraged}
  style="--tint:{h.tint}; background-image:url({arenaArt(arenaOf(hour))})"
  data-testid="raid-card" data-key={row.key} data-state={b.state}>
  <header>
    <span class="project" data-testid="raid-project">{label.project}</span>
    {#if row.hero}<span class="chip" data-testid="raid-hero">Lv {row.hero.level} {row.hero.title}</span>{/if}
    {#if row.external}<span class="chip ext" data-testid="raid-external">external</span>{/if}
  </header>
  <div class="task" data-testid="raid-task">{label.task}</div>
  {#if badges.list.length || pouch}
    <div class="badges">
      {#each badges.list as bd (bd.id)}<span class="badge" title={bd.name} data-testid="raid-badge">{bd.glyph}</span>{/each}
      {#if badges.more}<span class="badge more" data-testid="raid-badge-more">+{badges.more}</span>{/if}
      {#if pouch}<span class="pouch" class:dim={pouch.dim} style="--fill:{pouch.fill}" title="Quota used {Math.round(quotaPct)}%" data-testid="raid-pouch" data-dim={String(pouch.dim)}>◍</span>{/if}
    </div>
  {/if}

  <div class="stage">
    <div class="heroine pose-{b.pose}" data-testid="raid-heroine" data-pose={b.pose} data-hero={h.id} data-merc={String(h.mercenary)}
      data-look={look} data-tier={asc} title="{h.name}">
      {#if asc >= 1 && density !== "compact"}<span class="aura" class:pulse={mb.auraPulse} data-testid="raid-aura" data-pulse={String(mb.auraPulse)}></span>{/if}
      {#if asc >= 2 && density !== "compact"}<span class="ascend {wingsOf(h.id)}" data-testid="raid-halo" data-kind={wingsOf(h.id)}><Art src={haloArt(wingsOf(h.id))} tint={h.tint} quiet /></span>{/if}
      <Art src={heroineArt(h.id, b.pose, look, asc, raid.pack)} tint={h.tint} />
      {#each gear as g, i (g)}
        <span class="gear g{i}" class:drift={mb.gearDrift} data-testid="raid-gear" data-kind={g} data-drift={String(mb.gearDrift)}>
          <Art src={gearArt(g, h.id, raid.pack)} tint={h.tint} quiet />
        </span>
      {/each}
      {#if armour}<span class="armour" data-testid="raid-armour" data-pips={armour}>{"◆".repeat(armour)}</span>{/if}
    </div>
    {#if radar}<div class="radar-wrap" class:always={density === "hero"}><Radar r={radar} tint={h.tint} /></div>{/if}

    {#if density !== "compact" && b.boss !== "absent" && (animate || b.boss !== "dissolve")}
      <div class="boss boss-{b.boss}" class:elite={row.hotspot} data-testid="raid-boss" data-kind={kind} data-boss={b.boss}>
        {#if tier.tier === "slime"}
          <div class="slimes">{#each [0, 1, 2] as i (i)}<Art src={slimeArt(i)} tint="#6c6" cls="boss-img" testid="raid-slime" />{/each}</div>
        {:else}
          <Art src={bossArt(kind)} tint="#a55" cls="boss-img" />
        {/if}
        {#key hitAt}{#if animate && freshHit && b.state === "attack"}<span class="flash"></span><span class="dmg" data-testid="raid-dmg">−7</span>{/if}{/key}
        <div class="hp" data-testid="raid-hp" data-hp={b.hp}><span style="width:{b.hp}%"></span></div>
        <div class="name" data-testid="raid-boss-name">{BOSS_NAMES[kind]} · Lv {level}</div>
        {#if tier.pips}<span class="pips" data-testid="raid-pips" data-pips={tier.pips}>{"▮".repeat(tier.pips)}</span>{/if}
        {#if row.hotspot}<span class="tag elite-tag" data-testid="raid-elite">Elite · {target}</span>{/if}
        {#if b.enraged}<span class="tag rage" data-testid="raid-enraged">ENRAGED</span>{/if}
        {#if minions}<div class="minions">{#each Array(minions) as _, i (i)}<Art src={slimeArt(i)} tint="#6c6" cls="minion" testid="raid-minion" />{/each}</div>{/if}
      </div>
    {/if}

    {#if density === "compact" && b.boss !== "absent"}
      <!-- compact: no boss art, but HP and the elite mark still read -->
      <div class="cbar">
        <div class="hp" data-testid="raid-hp" data-hp={b.hp}><span style="width:{b.hp}%"></span></div>
        {#if row.hotspot}<span class="tag elite-tag" data-testid="raid-elite">Elite</span>{/if}
      </div>
    {/if}
    {#if b.state === "orders"}
      <div class="window" data-testid="raid-orders">
        <span>Your orders, Commander?</span>
        <span class="patience" data-testid="raid-patience" data-stage={b.patience}>{b.patience === "asleep" ? "zzz" : b.patience === "sitting" ? "sitting down" : "tapping her foot"}</span>
        {#if canOpen}<button data-nav data-testid="raid-review" onclick={() => onopen(row, el)}>▶ REVIEW</button>{/if}
      </div>
    {/if}
    {#if b.inn}<div class="inn" data-testid="raid-inn"><Art src={innArt()} tint="#c96" cls="inn-img" /><span>Resting at the inn</span></div>{/if}
  </div>

  {#if party.allies.length}
    <div class="allies">
      {#each party.allies as a (a.id)}
        <span class="ally {a.state}" class:away={a.background} style="--s:{a.size}" data-testid="raid-ally" data-kind={a.kind}
          title="{a.type || 'agent'} · {a.description || '(no description)'}{a.background ? ' — away on a quest' : ''}">
          <Art src={allyArt(a.kind, a.state === "working" || a.state === "tool" ? "cast" : "idle")} tint={h.tint} />
        </span>
      {/each}
      {#if party.more > 0}<span class="more" data-testid="raid-more">+{party.more}</span>{/if}
    </div>
  {/if}

  <footer>
    {#if b.state === "attack"}<span class="move" data-testid="raid-move">{moveOf(row.tool)}{target ? " · " + target : ""}</span>{/if}
    {#if b.state === "charge"}<span class="move">Charging…</span>{/if}
    {#if b.state === "rest"}<span class="rest" data-testid="raid-rest">Resting</span>{/if}
    {#if b.refreshed}<span class="tag" data-testid="raid-refreshed">Refreshed</span>{/if}
    {#if b.bag != null}<span class="bag" class:heavy={b.overburdened} data-testid="raid-bag">Loot {b.bag}%{b.overburdened ? " · Overburdened" : ""}</span>{/if}
    {#if canOpen && b.state !== "orders" && b.state !== "ko"}<button class="open" data-nav data-testid="raid-open" onclick={() => onopen(row, el)}>Open</button>{/if}
  </footer>

  {#if b.state === "clear"}
    <div class="banner clear" data-testid="raid-clear">
      <Art src={chestArt()} tint="#db3" cls="chest" />
      <strong>QUEST CLEAR</strong>
      {#if loot}
        <span class="loot" data-testid="raid-loot">
          {#each Array(loot.gems) as _, i (i)}<span class="gem">◆</span>{/each}{#if loot.chest}<span class="chestmark">▣</span>{/if}{#each Array(loot.scrolls) as _, i (i)}<span class="scroll">§</span>{/each}
          {loot.text}
        </span>
      {:else}
        <span>{row.turns ?? 0} turns{row.ctxPct != null ? ` · context ${row.ctxPct}%` : ""}</span>
      {/if}
    </div>
  {/if}
  {#if b.state === "ko"}
    <div class="banner ko" data-testid="raid-ko">
      <strong>K.O.</strong>
      {#if canOpen}<button data-nav data-testid="raid-revive" onclick={() => onopen(row, el)}>Revive</button>{/if}
    </div>
  {/if}
</article>

<style>
  .card {
    position: relative; display: flex; flex-direction: column; gap: 6px; padding: var(--sp-3); min-width: 0; overflow: hidden;
    border: 1px solid var(--border-0); border-radius: var(--r-3); background-color: var(--surface-1);
    background-size: cover; background-position: center; color: #f4f1ea;
  }
  .card::before { content: ""; position: absolute; inset: 0; background: linear-gradient(180deg, rgba(10, 12, 20, 0.72), rgba(10, 12, 20, 0.15) 45%, rgba(10, 12, 20, 0.78)); pointer-events: none; }
  .card > * { position: relative; }
  .card.external { opacity: 0.8; }
  .st-orders { border-color: var(--warn); }
  .st-ko { border-color: var(--crit); }
  header { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .project { font-weight: 700; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .task { font-size: var(--fs-1); opacity: 0.9; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
  .chip { font-size: var(--fs-0); background: rgba(0, 0, 0, 0.45); border-radius: var(--r-1); padding: 0 5px; white-space: nowrap; }
  .chip.ext { border: 1px dashed rgba(255, 255, 255, 0.5); }
  .stage { position: relative; display: grid; grid-template-columns: 1fr 1fr; align-items: end; height: 190px; }
  .hero .stage { height: 360px; }
  .compact .stage { height: 90px; grid-template-columns: 1fr; }
  .heroine, .boss { position: relative; height: 100%; display: flex; align-items: flex-end; justify-content: center; }
  .heroine { filter: drop-shadow(0 0 6px color-mix(in srgb, var(--tint) 60%, transparent)); }
  .heroine::after { content: ""; position: absolute; inset: 15% 10% 0; border-radius: 50%; opacity: 0; pointer-events: none;
    background: radial-gradient(circle, color-mix(in srgb, var(--tint) 45%, transparent), transparent 70%); }
  .st-attack .heroine { animation: lunge 900ms ease-in-out infinite; }
  .st-charge .heroine::after { animation: charge 2s ease-in-out infinite; }
  .boss-dissolve { animation: dissolve 1.2s ease-out 1 forwards; }
  .enraged .boss :global(.boss-img) { filter: sepia(1) saturate(4) hue-rotate(-30deg); }
  .elite { filter: drop-shadow(0 0 10px #f5c542); }
  .armour { position: absolute; left: 4px; bottom: 4px; color: #9cd3ff; font-size: 12px; letter-spacing: 1px; }
  .slimes, .minions { display: flex; align-items: flex-end; gap: 2px; height: 50%; }
  .minions { position: absolute; left: 0; bottom: 0; height: 28px; }
  .minions :global(.minion) { height: 28px; width: auto; }
  .flash { position: absolute; inset: 10%; border-radius: 50%; background: radial-gradient(circle, rgba(255, 240, 200, 0.8), transparent 70%); animation: fade 400ms ease-out 1 forwards; pointer-events: none; }
  .dmg { position: absolute; top: 10%; right: 20%; font-weight: 800; color: #ffd36b; text-shadow: 0 1px 2px #000; animation: float 900ms ease-out 1 forwards; }
  .hp { position: absolute; top: 0; left: 10%; right: 10%; height: 5px; background: rgba(0, 0, 0, 0.55); border-radius: 3px; overflow: hidden; }
  .hp span { display: block; height: 100%; background: linear-gradient(90deg, #d33, #f73); }
  .name { position: absolute; top: 8px; left: 0; right: 0; text-align: center; font-size: var(--fs-0); text-shadow: 0 1px 2px #000; }
  .pips { position: absolute; top: 22px; right: 10%; color: #f5c542; font-size: 10px; }
  .tag { font-size: var(--fs-0); background: rgba(0, 0, 0, 0.55); border-radius: var(--r-1); padding: 0 5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
  .boss .tag { position: absolute; left: 0; }
  .elite-tag { top: 24px; color: #f5c542; }
  .rage { top: 40px; color: #ff6b5b; font-weight: 700; }
  .window {
    position: absolute; right: 0; top: 20%; width: 50%; display: flex; flex-direction: column; gap: 4px; padding: 8px;
    background: rgba(12, 16, 40, 0.88); border: 2px solid #e8d9a8; border-radius: 6px; font-size: var(--fs-1);
  }
  .patience { font-size: var(--fs-0); opacity: 0.8; }
  /* compact: one slim line at the bottom of the stage, the heroine stays visible */
  .compact .window { top: auto; bottom: 0; width: 100%; flex-direction: row; align-items: center; gap: 6px; padding: 2px 6px; font-size: var(--fs-0); border-width: 1px; }
  .compact .window .patience { display: none; }
  .compact .window button { padding: 0 6px; font-size: var(--fs-0); }
  .cbar { position: absolute; left: 6px; right: 6px; top: 4px; display: flex; align-items: center; gap: 6px; }
  .cbar .hp { position: static; flex: 1; }
  .cbar .tag { position: static; }
  .window button, .banner button, .open {
    align-self: flex-start; background: rgba(0, 0, 0, 0.5); color: inherit; border: 1px solid #e8d9a8; border-radius: var(--r-2);
    padding: 2px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  .inn { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; background: rgba(20, 14, 8, 0.75); }
  .inn :global(.inn-img) { height: 60%; width: auto; }
  .allies { display: flex; align-items: flex-end; gap: 4px; min-height: 34px; }
  .ally { height: calc(34px * var(--s)); width: calc(28px * var(--s)); display: flex; }
  .ally.away { opacity: 0.4; }
  .ally.finished { animation: bloom 600ms ease-out 1; opacity: 0.6; }
  .more { font-size: var(--fs-0); opacity: 0.8; }
  footer { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: var(--fs-0); }
  .move { font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; max-width: 100%; }
  .bag.heavy { color: #ffb15b; }
  .open { margin-left: auto; }
  .banner { position: absolute; left: 0; right: 0; bottom: 30%; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 6px; background: rgba(0, 0, 0, 0.6); }
  .banner strong { letter-spacing: 0.12em; }
  .banner :global(.chest) { height: 32px; width: auto; }
  .ko strong { color: #ff6b5b; }
  .clear strong { color: #f5c542; }
  .aura { position: absolute; inset: 5% 0 0; border-radius: 50%; pointer-events: none; opacity: 0.55;
    background: radial-gradient(circle, color-mix(in srgb, var(--tint) 50%, transparent), transparent 68%); }
  .aura.pulse { animation: aura 3s ease-in-out infinite; }
  .ascend { position: absolute; pointer-events: none; display: flex; justify-content: center; }
  .ascend.halo { top: -2%; left: 30%; right: 30%; height: 22%; }
  .ascend.wings { top: 8%; left: 0; right: 0; height: 60%; opacity: 0.85; }
  .heroine > :global(.art) { position: relative; }
  .gear { position: absolute; width: 26%; height: 22%; display: flex; pointer-events: none; }
  .g0 { left: -2%; top: 18%; } .g1 { right: -2%; top: 8%; } .g2 { right: 4%; top: 42%; }
  .gear.drift { animation: drift 4s ease-in-out infinite; }
  .gear.g1.drift { animation-delay: -1.3s; } .gear.g2.drift { animation-delay: -2.6s; }
  .badges { display: flex; align-items: center; gap: 4px; font-size: var(--fs-0); }
  .badge { background: rgba(0, 0, 0, 0.5); border: 1px solid rgba(245, 197, 66, 0.6); color: #f5c542; border-radius: 50%; min-width: 18px; height: 18px; display: inline-flex; align-items: center; justify-content: center; }
  .badge.more { border-radius: var(--r-1); padding: 0 4px; color: inherit; }
  .pouch { margin-left: auto; color: #f5c542; opacity: calc(0.35 + 0.65 * var(--fill)); }
  .pouch.dim { color: #b9a37a; }
  .radar-wrap { position: absolute; left: 50%; top: 4px; transform: translateX(-50%); display: none; pointer-events: none; }
  .card:hover .radar-wrap, .card:focus-within .radar-wrap, .radar-wrap.always { display: block; }
  .loot { display: inline-flex; align-items: center; gap: 2px; flex-wrap: wrap; }
  .gem { color: #7fd3ff; } .chestmark { color: #f5c542; } .scroll { color: #e8d9a8; }
  @keyframes aura { 50% { opacity: 0.9; } }
  @keyframes drift { 50% { transform: translateY(-6px); } }
  @keyframes lunge { 50% { transform: translateX(14px); } }
  @keyframes charge { 50% { opacity: 1; } }
  @keyframes dissolve { to { opacity: 0; transform: scale(0.9); } }
  @keyframes fade { to { opacity: 0; } }
  @keyframes float { to { opacity: 0; transform: translateY(-24px); } }
  @keyframes bloom { from { transform: scale(1.4); opacity: 1; } }
</style>
