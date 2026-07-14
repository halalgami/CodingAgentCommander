<script>
  import Drawer from "./Drawer.svelte";
  import { app, saveKey, clearKey } from "../stores.svelte.js";

  let inputs = $state({});   // env -> pending value (drawer-local per spec)
  let errors = $state({});   // env -> inline error

  async function save(env) {
    errors[env] = "";
    if (!(inputs[env] || "").trim()) return;
    try {
      await saveKey(env, inputs[env]);
      inputs[env] = "";
    } catch (e) { errors[env] = "" + e; }
  }
  async function clear(env) {
    errors[env] = "";
    try { await clearKey(env); } catch (e) { errors[env] = "" + e; }
  }
</script>

<Drawer title="PROVIDERS" testid="drawer-providers" onclose={() => (app.drawer = null)}>
  {#each app.keys as k (k.env)}
    <div class="prov">
      <label>
        <span class="env mono">{k.env}</span>
        {#if k.optional}<span class="opt">optional</span>{/if}
        <span class="status" class:set={k.set}>{k.set ? "● set" : "○ unset"}</span>
      </label>
      <div class="row">
        <input
          type="password" data-testid="key-input-{k.env}"
          placeholder={k.optional ? "optional" : "paste key"}
          bind:value={inputs[k.env]}
          onkeydown={(e) => e.key === "Enter" && save(k.env)}
        />
        <button data-testid="key-save-{k.env}" onclick={() => save(k.env)}>Save</button>
        {#if k.set}<button class="danger" onclick={() => clear(k.env)}>Clear</button>{/if}
      </div>
      {#if errors[k.env]}<p class="err">{errors[k.env]}</p>{/if}
    </div>
  {/each}
  {#if app.keys.length === 0}
    <p class="dim">No routed providers in config.</p>
  {/if}
</Drawer>

<style>
  .prov { margin-bottom: var(--sp-4); }
  label { display: flex; gap: var(--sp-2); align-items: baseline; margin-bottom: var(--sp-1); font-size: var(--fs-1); }
  .env { color: var(--text-0); }
  .mono { font-family: var(--font-mono); }
  .opt { color: var(--text-2); font-size: var(--fs-0); }
  .status { margin-left: auto; color: var(--text-2); font-size: var(--fs-0); }
  .status.set { color: var(--ok); }
  .row { display: flex; gap: var(--sp-1); }
  input {
    flex: 1; min-width: 0; background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 6px 8px;
  }
  input:focus { outline: none; border-color: var(--accent-dim); }
  button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  button:hover { background: var(--surface-3); }
  .danger:hover { border-color: var(--crit); color: var(--crit); }
  .err { color: var(--crit); font-size: var(--fs-1); margin: var(--sp-1) 0 0; }
  .dim { color: var(--text-2); }
</style>
