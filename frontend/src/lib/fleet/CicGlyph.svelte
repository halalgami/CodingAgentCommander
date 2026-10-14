<!-- frontend/src/lib/fleet/CicGlyph.svelte — vector ship-class glyph, 16 units, centred on (0,0). -->
<script>
  let { ship = "opus", tint = "#9fe8a8", dashed = false } = $props();
  const PATHS = {
    opus: "M-8,-3 H6 L8,0 L6,3 H-8 Z M-6,-6 H2 V-3 H-6 Z M-6,3 H2 V6 H-6 Z",
    sonnet: "M-8,-4 L8,0 L-8,4 L-5,0 Z",
    haiku: "M-7,-5 L8,0 L-7,5 L-3,0 Z",
    glm: "M0,-7 L7,6 H-7 Z",
    qwen: "M0,-3 A3,3 0 1 1 -0.01,-3 Z M0,-3 V-8 M-6,5 H6",
    kimi: "M-8,-4 H8 V-1 H-8 Z M-8,1 H8 V4 H-8 Z",
  };
</script>

<path d={PATHS[ship] || PATHS.opus} fill="none" stroke={tint} stroke-width="1.6" stroke-dasharray={dashed ? "2 2" : null} stroke-linejoin="round" />
