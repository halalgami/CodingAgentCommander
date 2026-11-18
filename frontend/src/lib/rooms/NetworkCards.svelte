<script>
  // D10: the user picks the network — two cards, nothing preselected.
  import { rooms } from "./rooms.svelte.js";
  let { onpick, value = "" } = $props();
  const tsUp = $derived(!!rooms.status?.tailscaleUp);
  const CARDS = [
    ["lan", "Same network (Local)", "Both machines are on this Wi-Fi or LAN."],
    ["tailscale", "Over the internet (Tailscale)", "Both machines run Tailscale."],
  ];
</script>

<div class="cards" role="radiogroup" aria-label="How the machines connect">
  {#each CARDS as [net, title, body]}
    <button class="card" role="radio" data-testid="net-card" aria-checked={value === net} disabled={net === "tailscale" && !tsUp} onclick={() => onpick(net)}>
      <strong>{title}</strong><span>{body}</span>
    </button>
  {/each}
</div>
{#if !tsUp}<p class="dim">Tailscale isn't running on this machine.</p>{/if}

<style>
  .cards { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-2); }
  .card { display: flex; flex-direction: column; gap: 4px; text-align: left; padding: var(--sp-2); border-radius: var(--r-2); cursor: pointer;
    background: var(--surface-1); border: 1px solid var(--border-1); color: var(--text-0); font-size: var(--fs-1); }
  .card span { color: var(--text-2); font-size: var(--fs-0); }
  .card:hover:not(:disabled), .card:focus-visible { border-color: var(--accent); }
  .card[aria-checked="true"] { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent); }
  .card:disabled { opacity: 0.5; cursor: not-allowed; }
  .dim { color: var(--text-2); font-size: var(--fs-0); margin: 4px 0 0; }
</style>
