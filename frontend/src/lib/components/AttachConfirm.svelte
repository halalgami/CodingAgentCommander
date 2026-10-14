<script>
  import { confirmation, CONFIRM_MAX_ITEMS } from "../attachments.svelte.js";
  const items = $derived(confirmation.items);
  const shown = $derived(items.slice(0, CONFIRM_MAX_ITEMS));
  // A converted copy is shown under the name the user chose, not convert-<ts>.jpg.
  const shownName = (a) => (a.originalName ? `${a.originalName} (converted)` : a.name);
  const more = $derived(items.length - shown.length);
</script>

<!-- Rendered as the first child of Toast.svelte's .toasts stack: same layer
     (var(--layer-toast)), same corner, pointer-events: none inherited — so it
     never covers or fights the toasts. Names are user data; <bdi> keeps a
     bidi control-free but RTL name from reordering the sentence around it. -->
{#if items.length}
  <div class="confirm" data-testid="attach-confirm" role="status" aria-live="polite">
    <div class="title" data-testid="attach-confirm-title">
      {#if items.length === 1}Inserted <bdi>{shownName(items[0])}</bdi> into <bdi>{confirmation.session}</bdi>{:else}Inserted {items.length} paths into <bdi>{confirmation.session}</bdi>{/if}
    </div>
    <div class="row">
      {#each shown as a (a.id)}
        <span class="item">
          {#if a.kind === "image" && a.thumb}<img src={a.thumb} alt="" />{:else}<span class="icon" aria-hidden="true">{a.kind === "folder" ? "▣" : "▤"}</span>{/if}
          <bdi class="name" data-testid="attach-name">{shownName(a)}</bdi>
        </span>
      {/each}
      {#if more > 0}<span class="more" data-testid="attach-more">+{more} more</span>{/if}
    </div>
  </div>
{/if}

<style>
  .confirm {
    background: var(--surface-2); border: 1px solid var(--accent-dim);
    border-radius: var(--r-2); padding: var(--sp-2) var(--sp-3);
    max-width: 340px; box-shadow: 0 8px 24px oklch(0% 0 0 / 0.4);
  }
  .title { font-size: var(--fs-1); color: var(--text-0); margin-bottom: var(--sp-1); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-2); align-items: center; }
  .item { display: inline-flex; align-items: center; gap: var(--sp-1); max-width: 150px; }
  .item img { width: 32px; height: 32px; object-fit: cover; border-radius: var(--r-1); }
  .icon { width: 32px; text-align: center; color: var(--text-2); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-0); color: var(--text-1); }
  .more { font-size: var(--fs-0); color: var(--text-2); }
</style>
