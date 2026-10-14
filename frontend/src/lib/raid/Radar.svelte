<!-- frontend/src/lib/raid/Radar.svelte — 4-axis stat radar (Blade up, Scholar right, Engineer down, Summoner left). -->
<script>
  let { r, tint = "#ccc", testid = "raid-radar" } = $props();
  const R = 26, C = 32;
  const pct = (v) => Math.round(v * 100) + "%";
  const pts = $derived([[0, -r.blade], [r.scholar, 0], [0, r.engineer], [-r.summoner, 0]].map(([x, y]) => `${C + x * R},${C + y * R}`).join(" "));
  const label = $derived(`Blade ${pct(r.blade)}, Scholar ${pct(r.scholar)}, Engineer ${pct(r.engineer)}, Summoner ${pct(r.summoner)}`);
</script>

<svg class="radar" viewBox="0 0 64 64" width="64" height="64" role="img" aria-label={label} data-testid={testid}>
  <polygon points="32,6 58,32 32,58 6,32" fill="rgba(0,0,0,0.5)" stroke="rgba(255,255,255,0.35)" />
  <line x1="32" y1="6" x2="32" y2="58" stroke="rgba(255,255,255,0.2)" /><line x1="6" y1="32" x2="58" y2="32" stroke="rgba(255,255,255,0.2)" />
  <polygon points={pts} fill={tint} fill-opacity="0.55" stroke={tint} />
</svg>
