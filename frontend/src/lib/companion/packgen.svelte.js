// Wizard state and every Wails call it makes.
//
// Two rules shape this file.
//
// The RUN lives in Go, not here. It survives the wizard being closed, the
// drawer being switched, and the deck being reloaded. So the wizard never
// stores progress of its own — it renders `gen.run`, which is a mirror of
// Go's state, refreshed by events and by an explicit poll on open.
//
// The PHASE is DERIVED from the run wherever the run has an opinion. Storing a
// phase independently is the bug that makes reopening the wizard mid-run show
// step 1 with a "Generate" button, over a run that is already spending money.

import {
  StartPackGen, CancelPackGen, SavePackGen, DiscardPackGen, PackGenStatus,
  PackGenStyles, PackGenSlots, PackGenProviders, PackGenConditions,
  PackGenEstimate, PickPackGenBase, InspectPackGenBase, SetImagegenKey, ClearImagegenKey,
  BrowseCompanionPack, SetPackVariantFocus, ListCompanionPacks, SelectCompanionPack,
  ExportCompanionPack, ImportCompanionPack, DeleteCompanionPack,
} from "../../../wailsjs/go/main/App.js";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { toast, companionLoadPack } from "../stores.svelte.js";
import {
  PHASES, slotKey, phaseFor, selectedSlotsFrom, defaultPicks,
  conditionsFor as conditionsForIn, nextModel,
  packModel, addableScenes, selectedAddSlots,
} from "./packgen-logic.js";

// Everything that DECIDES lives in packgen-logic.js, which is rune-free and
// therefore unit-testable; this module holds the state and makes the calls.
export { PHASES, slotKey };

export const gen = $state({
  open: false,
  /** Mirror of Go's PackGenState. null when no run exists. */
  run: null,
  /** The user's step, consulted ONLY when the run has no opinion. */
  step: "base",

  // catalogue, loaded once when the wizard opens
  styles: [],
  slotDefaults: [],
  providers: [],
  conditions: [],

  // the form
  name: "",
  providerID: "",
  model: "",
  styleID: "",
  base: null, // { path, w, h, warning }
  /** slot key ("idle" or "bored|lateNight") -> { on, staging } */
  picks: {},

  cost: null,
  keyInput: "",
  /** The SAVED pack, for the browser. null until loaded. */
  browse: null,
  /** Every pack in the packs folder. */
  library: [],
  /** slotKey of the variant whose details are expanded, or "". */
  openVariant: "",
  /** The pack awaiting a delete confirmation, or null. Deleting art is the one
   *  irreversible thing here, so it never happens on a single click. */
  confirmDelete: null,
  /** Set to Go's refusal message when a new pack would replace an existing
   *  one. Drives a confirmation rather than a dead end. */
  confirmOverwrite: null,
  /** Per-variant prompt edits, keyed by slotKey; unset means "as saved". */
  promptEdits: {},
  /** slotKey -> { on, staging } for the "Add scenes" picker. */
  addPicks: {},
  /** Cost estimate for the current add selection, or null. */
  addCost: null,
  busy: false,
  error: "",
});

export const currentPhase = () => phaseFor(gen.run, gen.step);
export const selectedSlots = () => selectedSlotsFrom(gen.picks);
export const conditionsFor = (slot) => conditionsForIn(gen.conditions, slot);

export async function openPackGen() {
  gen.open = true;
  gen.error = "";
  try {
    const [styles, slots, providers, conditions, run] = await Promise.all([
      PackGenStyles(), PackGenSlots(), PackGenProviders(), PackGenConditions(), PackGenStatus(),
    ]);
    gen.styles = styles ?? [];
    gen.slotDefaults = slots ?? [];
    gen.providers = providers ?? [];
    gen.conditions = conditions ?? [];
    gen.run = run?.id ? run : null;

    if (!gen.styleID) gen.styleID = gen.styles[0]?.id ?? "";
    if (!gen.providerID) {
      // Prefer a provider that already has a key: it is the one the user can
      // actually generate with, and defaulting elsewhere sends them to a key
      // form they did not need.
      const withKey = gen.providers.find((p) => p.hasKey);
      gen.providerID = (withKey ?? gen.providers[0])?.id ?? "";
    }
    syncModel();
    if (!Object.keys(gen.picks).length) resetPicks();
    await Promise.all([refreshCost(), refreshBrowse(), refreshLibrary()]);
    // The picker rows come from the global catalogue, not this pack, so a
    // selection left over from a previous session (or a previous pack, if the
    // wizard is reopened without a full reload) would silently carry over.
    gen.addPicks = {};
    gen.addCost = null;
  } catch (e) {
    // In a plain browser (Playwright) there is no Wails runtime; the wizard
    // must still mount so its layout can be tested.
    gen.error = "";
  }
}

export function closePackGen() {
  // Closing does NOT cancel. The run is Go-side and keeps going; that is the
  // whole reason this is a panel and not a modal.
  gen.open = false;
}

/** Ticks every base slot on with its default staging, conditions off. */
export function resetPicks() {
  gen.picks = defaultPicks(gen.slotDefaults);
}

export function togglePick(slot, when, on) {
  const key = slotKey(slot, when);
  const existing = gen.picks[key];
  if (existing) {
    existing.on = on;
  } else {
    const base = gen.slotDefaults.find((s) => s.slot === slot);
    gen.picks[key] = { on, staging: base?.staging ?? "" };
  }
  refreshCost();
}

export function setStaging(slot, when, text) {
  const key = slotKey(slot, when);
  if (gen.picks[key]) gen.picks[key].staging = text;
}

export function syncModel() {
  gen.model = nextModel(gen.providers, gen.providerID, gen.model);
}

export function activeProvider() {
  return gen.providers.find((p) => p.id === gen.providerID) ?? null;
}

export function askDeletePack(p) { gen.confirmDelete = p; }
export function cancelDeletePack() { gen.confirmDelete = null; }

/** Delete the pack the user confirmed. Never called from a click handler
 *  directly — only from the confirmation. */
export async function confirmDeletePack() {
  const p = gen.confirmDelete;
  if (!p) return;
  gen.error = "";
  gen.busy = true;
  try {
    await DeleteCompanionPack(p.path);
    gen.confirmDelete = null;
    // Both views plus the figure: deleting the ACTIVE pack clears the
    // selection Go-side, so the sidebar has to be told as well.
    await Promise.all([refreshBrowse(), refreshLibrary(), companionLoadPack()]);
    // The active pack's identity may have just changed (deleting the active
    // pack clears it Go-side); a leftover add-picker selection would otherwise
    // apply to whatever pack ends up active next.
    gen.addPicks = {};
    gen.addCost = null;
    toast("Deleted " + p.name);
  } catch (e) {
    gen.error = "" + e;
    gen.confirmDelete = null;
  } finally {
    gen.busy = false;
  }
}

export async function refreshLibrary() {
  try { gen.library = (await ListCompanionPacks()) ?? []; } catch { gen.library = []; }
}

/** Switch the active pack, then re-read both views so neither goes stale. */
export async function selectPack(p) {
  gen.error = "";
  try {
    await SelectCompanionPack(p.path);
    await Promise.all([refreshBrowse(), refreshLibrary(), companionLoadPack()]);
    // The add-picker's keys are global (slot|when), so a selection ticked
    // against the OLD pack would otherwise silently apply to the new one —
    // either billing a scene the user never meant to add, or firing a no-op
    // regen that holds the slot until Discard.
    gen.addPicks = {};
    gen.addCost = null;
    toast("Companion pack: " + p.name);
  } catch (e) { gen.error = "" + e; }
}

/** Back the active pack up to a .zip the user chooses. */
export async function exportPack() {
  gen.error = "";
  try {
    const dest = await ExportCompanionPack();
    if (dest) toast("Exported to " + dest.split("/").pop());
  } catch (e) { gen.error = "" + e; }
}

/** Add a pack from a .zip. Never overwrites an existing one — Go picks a free
 *  folder — so this is always additive. */
export async function importPack() {
  gen.error = "";
  try {
    const name = await ImportCompanionPack();
    if (!name) return;                       // cancelled
    await Promise.all([refreshBrowse(), refreshLibrary(), companionLoadPack()]);
    // An import makes the imported pack active, i.e. a pack-identity change —
    // same reasoning as selectPack.
    gen.addPicks = {};
    gen.addCost = null;
    toast("Imported " + name);
  } catch (e) { gen.error = "" + e; }
}

export async function refreshBrowse() {
  try {
    gen.browse = await BrowseCompanionPack();
  } catch {
    // The only error Go raises is "no pack configured", which is a state the
    // browser renders rather than an failure to report.
    gen.browse = null;
  }
}

/** The prompt that will actually be sent for a variant: edited, or as saved. */
export function promptFor(v) {
  const key = slotKey(v.slot, v.when);
  return gen.promptEdits[key] ?? v.prompt ?? "";
}

export function setPromptFor(v, text) {
  gen.promptEdits[slotKey(v.slot, v.when)] = text;
}

/**
 * Regenerate ONE saved variant. The prompt shown is the one sent — no silent
 * recomposition — so what the user edits is exactly what they pay for.
 */
export async function regenerateVariant(v) {
  gen.error = "";
  gen.busy = true;
  try {
    gen.run = await StartPackGen({
      name: gen.browse?.name || "",
      // The PATH, not the name. The folder is slug(name) only for packs this
      // app generated: an imported pack keeps its manifest name but takes its
      // folder from the archive filename, so addressing by name regenerated a
      // DIFFERENT pack and rewrote that one.
      packPath: gen.browse?.path ?? "",
      providerId: gen.providerID,
      // The variant's OWN model, not whatever the Create tab happens to be
      // showing. Re-rolling one scene with a different model puts a stranger
      // in a set whose entire premise is that it is one character.
      model: v.model || gen.model,
      styleId: gen.styleID,
      basePath: "",              // Go takes it from inside the pack
      regenerate: true,
      slots: [{ slot: v.slot, when: v.when, staging: "", prompt: promptFor(v) }],
    });
  } catch (e) {
    gen.error = "" + e;
  } finally {
    gen.busy = false;
  }
}

/** Nudge a saved variant's crop. Cheap where regenerating costs an image. */
export async function nudgeFocus(v, focusX, focusY) {
  gen.error = "";
  try {
    // Mood is part of the identity: a slot may hold several variants with the
    // same condition, and matching on condition alone moved the wrong row.
    await SetPackVariantFocus(v.slot, v.when, v.mood ?? "", focusX, focusY);
    await Promise.all([refreshBrowse(), refreshLibrary(), companionLoadPack()]);
  } catch (e) { gen.error = "" + e; }
}

/** slot|when keys already in the saved pack — never addable. */
function addPresentKeys() {
  return new Set(
    addableScenes(gen.slotDefaults, gen.conditions, gen.browse)
      .filter((r) => r.present)
      .map((r) => slotKey(r.slot, r.when)),
  );
}

/** The model an added scene will use: the pack's, resolved to one the selected
 *  provider actually offers (else its first), falling back to the Create-tab
 *  model. nextModel keeps it valid for gen.providerID. */
function addModel() {
  return nextModel(gen.providers, gen.providerID, packModel(gen.browse) || gen.model);
}

export function toggleAddPick(slot, when, on) {
  const key = slotKey(slot, when);
  const existing = gen.addPicks[key];
  if (existing) {
    existing.on = on;
  } else {
    const base = gen.slotDefaults.find((s) => s.slot === slot);
    gen.addPicks[key] = { on, staging: base?.staging ?? "" };
  }
  refreshAddCost();
}

export function setAddStaging(slot, when, text) {
  const key = slotKey(slot, when);
  if (gen.addPicks[key]) gen.addPicks[key].staging = text;
}

export async function refreshAddCost() {
  const n = selectedAddSlots(gen.addPicks, addPresentKeys()).length;
  const model = addModel();
  if (!model || n === 0) { gen.addCost = null; return; }
  try { gen.addCost = await PackGenEstimate(model, n); } catch { gen.addCost = null; }
}

/**
 * Add the ticked scenes to the saved pack via the regenerate path, which upserts
 * on (slot,when,mood): an absent combo is appended and everything else survives.
 * A failure leaves the saved pack exactly as it was (staged copy, promote on
 * success). Addressed by PATH — an imported pack's folder is not slug(name).
 */
export async function addScenes() {
  gen.error = "";
  gen.busy = true;
  try {
    gen.run = await StartPackGen({
      name: gen.browse?.name || "",
      packPath: gen.browse?.path ?? "",
      providerId: gen.providerID,
      model: addModel(),
      styleId: gen.styleID,
      basePath: "",
      regenerate: true,
      slots: selectedAddSlots(gen.addPicks, addPresentKeys()),
    });
    gen.addPicks = {};
    gen.addCost = null;
  } catch (e) {
    gen.error = "" + e;
  } finally {
    gen.busy = false;
  }
}

export async function refreshCost() {
  const n = selectedSlots().length;
  if (!gen.model || n === 0) { gen.cost = null; return; }
  try { gen.cost = await PackGenEstimate(gen.model, n); } catch { gen.cost = null; }
}

export async function pickBase() {
  gen.error = "";
  try {
    const path = await PickPackGenBase();
    if (!path) return;                       // cancelled
    // PickPackGenBase already decoded the file, so a path coming back is known
    // good; this second call is only to read back the dimensions and warning.
    gen.base = await InspectPackGenBase(path);
  } catch (e) {
    gen.error = "" + e;
  }
}

export async function saveKey() {
  gen.error = "";
  const key = gen.keyInput.trim();
  if (!key) return;
  try {
    await SetImagegenKey(gen.providerID, key);
    gen.keyInput = "";                        // never keep it in webview memory
    gen.providers = await PackGenProviders();
    toast("Key saved");
  } catch (e) { gen.error = "" + e; }
}

export async function clearKey() {
  gen.error = "";
  try {
    await ClearImagegenKey(gen.providerID);
    gen.providers = await PackGenProviders();
  } catch (e) { gen.error = "" + e; }
}

/**
 * Start a new run. `overwrite` is only ever true when the user has confirmed
 * replacing an existing pack — Go refuses otherwise, because promotion deletes
 * what it replaces and the name is left pre-filled after a save.
 */
export async function startRun({ overwrite = false } = {}) {
  gen.error = "";
  gen.busy = true;
  try {
    gen.run = await StartPackGen({
      name: gen.name,
      providerId: gen.providerID,
      model: gen.model,
      styleId: gen.styleID,
      basePath: gen.base?.path ?? "",
      overwrite,
      slots: selectedSlots(),
    });
    gen.confirmOverwrite = null;
  } catch (e) {
    const msg = "" + e;
    // Go refuses the collision; the UI turns that into a decision rather than
    // a dead end.
    if (msg.includes("already exists")) {
      gen.confirmOverwrite = msg;
    } else {
      gen.error = msg;
    }
  } finally {
    gen.busy = false;
  }
}

export function cancelOverwrite() { gen.confirmOverwrite = null; }

export async function cancelRun() {
  try { gen.run = await CancelPackGen(); } catch (e) { gen.error = "" + e; }
}

export async function saveRun() {
  gen.error = "";
  gen.busy = true;
  try {
    gen.run = await SavePackGen();
    // The pack is now the active one, so the sidebar figure must pick it up
    // without waiting for a restart.
    await Promise.all([companionLoadPack(), refreshBrowse(), refreshLibrary()]);
    toast("Pack saved");
    // Clear the name. Leaving it filled is what put the next run on a
    // collision course with the pack that was just saved.
    gen.name = "";
    gen.step = "base";
  } catch (e) {
    gen.error = "" + e;
  } finally {
    gen.busy = false;
  }
}

export async function discardRun() {
  gen.error = "";
  gen.busy = true;
  try {
    await DiscardPackGen();
    gen.run = null;
    gen.step = "base";
  } catch (e) {
    gen.error = "" + e;
  } finally {
    gen.busy = false;
  }
}

/**
 * Subscribes to Go's progress events. Called once at app start, NOT when the
 * wizard opens: the run continues while the panel is closed, and the state has
 * to be current the moment it reopens.
 */
export function initPackGen() {
  try {
    EventsOn("packgen:progress", (s) => { gen.run = s; });
  } catch {}
}
