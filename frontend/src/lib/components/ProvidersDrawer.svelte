<script>
  import Drawer from "./Drawer.svelte";
  import { app, saveKey, clearKey, addProvider, removeProvider, providerLabel } from "../stores.svelte.js";

  let inputs = $state({});
  let errors = $state({});
  let provErrors = $state({});
  let regionInput = $state("us-east-1");

  // key envs belonging to each provider type, in KeyStatus order
  const envsFor = (type) =>
    type === "opencode-go" ? ["ZEN_KEY"] : ["AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"];
  const keyInfo = (env) => app.keys.find((k) => k.env === env);

  async function define(p) {
    provErrors[p.type] = "";
    try { await addProvider(p.type, p.type === "opencode-go" ? p.apiBase : "", regionInput); }
    catch (e) { provErrors[p.type] = "" + e; }
  }
  async function undefine(p) {
    provErrors[p.type] = "";
    if (!confirm(`Remove ${providerLabel(p.type)}? Also removes its ${p.modelCnt} model(s).`)) return;
    try { await removeProvider(p.type); } catch (e) { provErrors[p.type] = "" + e; }
  }
  async function save(env) {
    errors[env] = "";
    if (!(inputs[env] || "").trim()) return;
    try { await saveKey(env, inputs[env]); inputs[env] = ""; } catch (e) { errors[env] = "" + e; }
  }
  async function clear(env) {
    errors[env] = "";
    try { await clearKey(env); } catch (e) { errors[env] = "" + e; }
  }
</script>

<Drawer title="PROVIDERS" testid="drawer-providers" onclose={() => (app.drawer = null)}>
  {#each app.providers as p (p.type)}
    <section class="ptype" data-testid="provider-{p.type}">
      <header>
        <span class="name">{providerLabel(p.type)}</span>
        <span class="status" class:set={p.active}>
          {p.active ? "● active" : p.defined ? "○ key missing" : "○ undefined"}
        </span>
      </header>
      {#if !p.defined}
        {#if p.type === "bedrock"}
          <input placeholder="region e.g. us-east-1" bind:value={regionInput} />
        {:else}
          <p class="dim mono">{p.apiBase}</p>
        {/if}
        <button class="primary" data-testid="define-{p.type}" onclick={() => define(p)}>Define</button>
      {:else}
        {#if p.type === "bedrock"}<p class="dim">region: <span class="mono">{p.region}</span></p>
        {:else}<p class="dim mono">{p.apiBase}</p>{/if}
        {#each envsFor(p.type) as env (env)}
          {@const k = keyInfo(env)}
          {#if k}
            <div class="prov">
              <label>
                <span class="env mono">{k.env}</span>
                {#if k.optional}<span class="opt">optional</span>{/if}
                <span class="status" class:set={k.set}>{k.set ? "● set" : "○ unset"}</span>
              </label>
              <div class="row">
                <input type="password" data-testid="key-input-{k.env}"
                  placeholder={k.optional ? "optional" : "paste key"}
                  bind:value={inputs[k.env]}
                  onkeydown={(e) => e.key === "Enter" && save(k.env)} />
                <button data-testid="key-save-{k.env}" onclick={() => save(k.env)}>Save</button>
                {#if k.set}<button class="danger" onclick={() => clear(k.env)}>Clear</button>{/if}
              </div>
              {#if errors[k.env]}<p class="err">{errors[k.env]}</p>{/if}
            </div>
          {/if}
        {/each}
        <button class="danger" data-testid="remove-{p.type}" onclick={() => undefine(p)}>Remove provider</button>
      {/if}
      {#if provErrors[p.type]}<p class="err">{provErrors[p.type]}</p>{/if}
    </section>
  {/each}
</Drawer>

<style>
  .ptype { margin-bottom: var(--sp-5); padding-bottom: var(--sp-3); border-bottom: 1px solid var(--border-0); }
  .ptype header { display: flex; justify-content: space-between; margin-bottom: var(--sp-2); }
  .ptype .name { color: var(--text-0); font-size: var(--fs-1); letter-spacing: 0.05em; }
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
  .primary { background: var(--accent); color: var(--on-accent); border: 0; font-weight: 600; }
  .primary:hover { background: var(--accent-hover); }
  .danger:hover { border-color: var(--crit); color: var(--crit); }
  .err { color: var(--crit); font-size: var(--fs-1); margin: var(--sp-1) 0 0; }
  .dim { color: var(--text-2); }
</style>
