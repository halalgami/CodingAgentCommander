<script>
  import Drawer from "./Drawer.svelte";
  import { app, addModel, removeModel, discoverBedrock, discoverZen, providerLabel } from "../stores.svelte.js";

  // per-section state, keyed by provider type
  let disc = $state({});        // type -> [{id,label,upstream?,region?}]
  let discSel = $state({});     // type -> {id: bool}
  let discBusy = $state({});    // type -> bool
  let secErr = $state({});      // type -> inline error
  let manual = $state({});      // type -> manual upstream/id input
  let native = $state({ id: "", label: "" });

  const definedProviders = $derived(app.providers.filter((p) => p.defined));
  const undefinedProviders = $derived(app.providers.filter((p) => !p.defined));

  async function remove(id) {
    try { await removeModel(id); } catch (e) { secErr.list = "" + e; }
  }

  async function addNative() {
    secErr.anthropic = "";
    try {
      await addModel({ id: native.id, label: native.label, provider: "anthropic",
        upstream: "", apiBase: "", keyEnv: "", region: "", inputPrice: 0, outputPrice: 0 });
      native = { id: "", label: "" };
    } catch (e) { secErr.anthropic = "" + e; }
  }

  async function discover(p) {
    secErr[p.type] = ""; discBusy[p.type] = true; disc[p.type] = []; discSel[p.type] = {};
    try {
      if (p.type === "bedrock") {
        disc[p.type] = await discoverBedrock(p.region);
        if (!disc[p.type].length) secErr[p.type] = "No invokable tool-capable models found (enable model access in the AWS console).";
      } else {
        disc[p.type] = (await discoverZen()).map((m) => ({ ...m, upstream: "openai/" + m.id }));
        if (!disc[p.type].length) secErr[p.type] = "No models returned.";
      }
    } catch (e) { secErr[p.type] = "" + e; }
    discBusy[p.type] = false;
  }

  async function addSelected(p) {
    secErr[p.type] = "";
    try {
      for (const m of disc[p.type] || []) {
        if (!discSel[p.type]?.[m.id]) continue;
        await addModel({ id: m.id, label: m.label, provider: p.type,
          upstream: m.upstream, apiBase: "", keyEnv: "",
          region: m.region || "", inputPrice: 0, outputPrice: 0 });
      }
      disc[p.type] = []; discSel[p.type] = {};
    } catch (e) { secErr[p.type] = "" + e; }
  }

  async function addManual(p) {
    secErr[p.type] = "";
    const up = (manual[p.type] || "").trim();
    if (!up) return;
    const id = up.replace(/^(openai|bedrock)\//, "").replace(/[.:\/ ]/g, "-").toLowerCase();
    try {
      await addModel({ id, label: id, provider: p.type,
        upstream: p.type === "opencode-go" && !up.includes("/") ? "openai/" + up : up,
        apiBase: "", keyEnv: "", region: "", inputPrice: 0, outputPrice: 0 });
      manual[p.type] = "";
    } catch (e) { secErr[p.type] = "" + e; }
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
  {#if secErr.list}<p class="err">{secErr.list}</p>{/if}

  <section data-testid="models-section-anthropic">
    <h3>Anthropic (native)</h3>
    <div class="form">
      <input data-testid="add-model-id" placeholder="model id e.g. claude-sonnet-5" bind:value={native.id} />
      <input placeholder="label" bind:value={native.label} />
      <button class="primary" data-testid="add-model-submit" onclick={addNative}>Add model</button>
      {#if secErr.anthropic}<p class="err" data-testid="add-model-error">{secErr.anthropic}</p>{/if}
    </div>
  </section>

  {#each definedProviders as p (p.type)}
    <section data-testid="models-section-{p.type}">
      <h3>{providerLabel(p.type)}</h3>
      <div class="form">
        <button data-testid="discover-{p.type === 'bedrock' ? 'bedrock' : 'zen'}"
          disabled={discBusy[p.type]} onclick={() => discover(p)}>
          {discBusy[p.type] ? "Discovering…" : "Discover models"}
        </button>
        {#if disc[p.type]?.length}
          <div class="disclist">
            {#each disc[p.type] as m (m.id)}
              <label class="discrow" class:anthropic={m.anthropic}>
                <input type="checkbox" bind:checked={discSel[p.type][m.id]} />
                <span>{m.label}</span>
              </label>
            {/each}
            <button onclick={() => addSelected(p)}>Add selected</button>
          </div>
        {/if}
        <details>
          <summary class="dim">Add manually</summary>
          <input data-testid="manual-upstream-{p.type}"
            placeholder={p.type === "bedrock" ? "bedrock/us.anthropic.claude-…" : "model id e.g. glm-5.2"}
            bind:value={manual[p.type]}
            onkeydown={(e) => e.key === "Enter" && addManual(p)} />
        </details>
        {#if secErr[p.type]}<p class="err">{secErr[p.type]}</p>{/if}
      </div>
    </section>
  {/each}

  {#each undefinedProviders as p (p.type)}
    <p class="dim hint" data-testid="models-hint-{p.type}">
      Define {providerLabel(p.type)} in Providers to add its models.
    </p>
  {/each}
</Drawer>

<style>
  .catalog { list-style: none; margin: 0 0 var(--sp-4); padding: 0; }
  .catalog li {
    display: flex; justify-content: space-between; align-items: center;
    padding: var(--sp-1) 0; border-bottom: 1px solid var(--border-0); font-size: var(--fs-1);
  }
  h3 { font-size: var(--fs-1); letter-spacing: 0.1em; color: var(--text-1); margin: 0 0 var(--sp-2); }
  section { margin-bottom: var(--sp-4); }
  .hint { margin: var(--sp-2) 0; }
  details summary { cursor: pointer; font-size: var(--fs-0); }
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
  .err { color: var(--crit); font-size: var(--fs-1); margin: 0; }
  .dim { color: var(--text-2); font-size: var(--fs-0); margin: 0; }
</style>
