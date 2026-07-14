<script>
  import Drawer from "./Drawer.svelte";
  import Select from "./Select.svelte";
  import { app, addModel, removeModel, discoverBedrock } from "../stores.svelte.js";

  const blank = () => ({ id: "", label: "", provider: "anthropic", upstream: "", apiBase: "", keyEnv: "", region: "", inputPrice: 0, outputPrice: 0 });
  let form = $state(blank());
  let formError = $state("");
  let discovered = $state([]);
  let discSelected = $state({});
  let discovering = $state(false);
  let discoverError = $state("");

  const providerOptions = [
    { value: "anthropic", label: "anthropic (native)" },
    { value: "opencode-go", label: "OpenCode Zen/Go (routed)" },
    { value: "bedrock", label: "bedrock (AWS)" },
  ];

  async function submit() {
    formError = "";
    try {
      await addModel(form);
      form = blank();
    } catch (e) { formError = "" + e; }
  }
  async function remove(id) {
    formError = "";
    try { await removeModel(id); } catch (e) { formError = "" + e; }
  }
  async function discover() {
    discoverError = ""; discovering = true; discovered = []; discSelected = {};
    try {
      discovered = await discoverBedrock((form.region || "us-east-1").trim());
      if (!discovered.length) discoverError = "No invokable text models found (enable model access in the AWS console).";
    } catch (e) { discoverError = "" + e; }
    discovering = false;
  }
  async function addSelected() {
    discoverError = "";
    const region = (form.region || "us-east-1").trim();
    try {
      for (const m of discovered) {
        if (!discSelected[m.id]) continue;
        await addModel({ id: m.id, label: m.label, provider: "bedrock", upstream: m.upstream, apiBase: "", keyEnv: "", region, inputPrice: 0, outputPrice: 0 });
      }
      discovered = []; discSelected = {};
    } catch (e) { discoverError = "" + e; }
  }
</script>

<Drawer title="MODELS" testid="drawer-models" onclose={() => (app.drawer = null)}>
  <ul class="catalog">
    {#each app.catalog as m (m.id)}
      <li>
        <span>{m.label || m.id} <span class="dim">{m.provider}</span></span>
        <button class="icon" title="Remove" onclick={() => remove(m.id)}>✕</button>
      </li>
    {/each}
  </ul>

  <h3>Add model</h3>
  <div class="form">
    <input data-testid="add-model-id" placeholder="id" bind:value={form.id} />
    <input placeholder="label" bind:value={form.label} />
    <Select options={providerOptions} bind:value={form.provider} testid="add-model-provider" />
    {#if form.provider === "bedrock"}
      <input placeholder="region e.g. us-east-1" bind:value={form.region} />
      <button data-testid="discover-button" disabled={discovering} onclick={discover}>
        {discovering ? "Discovering…" : "Discover models"}
      </button>
      {#if discoverError}<p class="err">{discoverError}</p>{/if}
      {#if discovered.length}
        <div class="disclist">
          {#each discovered as m (m.id)}
            <label class="discrow" class:anthropic={m.anthropic} class:noagent={!m.agentCapable}>
              <input type="checkbox" bind:checked={discSelected[m.id]} />
              <span>{m.label}</span>
              {#if !m.agentCapable}<span class="warn" title="No tool use via the Converse API — Claude Code's tool calls will fail">no tools</span>{/if}
            </label>
          {/each}
          <button onclick={addSelected}>Add selected</button>
        </div>
      {/if}
      <p class="dim">Or type one manually:</p>
      <input placeholder="upstream e.g. bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0" bind:value={form.upstream} />
      <p class="dim">AWS access key + secret set in Providers.</p>
    {:else if form.provider !== "anthropic"}
      <input placeholder="upstream e.g. openai/kimi-k2.5" bind:value={form.upstream} />
      <input placeholder="api_base" bind:value={form.apiBase} />
      <input placeholder="key_env e.g. ZEN_KEY" bind:value={form.keyEnv} />
    {/if}
    <button class="primary" data-testid="add-model-submit" onclick={submit}>Add model</button>
    {#if formError}<p class="err" data-testid="add-model-error">{formError}</p>{/if}
  </div>
</Drawer>

<style>
  .catalog { list-style: none; margin: 0 0 var(--sp-4); padding: 0; }
  .catalog li {
    display: flex; justify-content: space-between; align-items: center;
    padding: var(--sp-1) 0; border-bottom: 1px solid var(--border-0); font-size: var(--fs-1);
  }
  h3 { font-size: var(--fs-1); letter-spacing: 0.1em; color: var(--text-1); margin: 0 0 var(--sp-2); }
  .form { display: flex; flex-direction: column; gap: var(--sp-2); }
  input:not([type="checkbox"]) {
    background: var(--surface-2); color: var(--text-0);
    border: 1px solid var(--border-0); border-radius: var(--r-2); padding: 6px 8px;
  }
  input:focus { outline: none; border-color: var(--accent-dim); }
  button {
    background: var(--surface-2); color: var(--text-0); border: 1px solid var(--border-0);
    border-radius: var(--r-2); padding: 6px 10px; cursor: pointer; font-size: var(--fs-1);
  }
  button:hover:not(:disabled) { background: var(--surface-3); }
  button:disabled { opacity: 0.6; cursor: default; }
  .primary { background: var(--accent); color: var(--on-accent); border: 0; font-weight: 600; }
  .primary:hover { background: var(--accent-hover); }
  .icon { background: none; border: 0; color: var(--text-1); padding: 2px 4px; }
  .icon:hover { color: var(--crit); }
  .disclist {
    display: flex; flex-direction: column; gap: 2px; max-height: 200px; overflow-y: auto;
    background: var(--surface-0); border-radius: var(--r-2); padding: var(--sp-2);
  }
  .discrow { display: flex; align-items: center; gap: var(--sp-2); font-size: var(--fs-0); color: var(--text-1); cursor: pointer; }
  .discrow.anthropic span { color: var(--accent); }
  .discrow.noagent span { color: var(--text-2); }
  .discrow .warn {
    margin-left: auto; flex: none; font-size: var(--fs-0);
    color: var(--warn); border: 1px solid var(--border-0);
    border-radius: var(--r-1); padding: 0 4px;
  }
  .err { color: var(--crit); font-size: var(--fs-1); margin: 0; }
  .dim { color: var(--text-2); font-size: var(--fs-0); margin: 0; }
</style>
