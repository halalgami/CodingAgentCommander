import { test, expect } from "@playwright/test";

// The wizard's state is Go-fed at runtime, so these drive it through the
// __packgen seam the deck exposes — the same trick the other companion specs
// use for __app.

const SLOTS = [
  { slot: "idle", staging: "she waits" },
  { slot: "working", staging: "she works" },
  { slot: "bored", staging: "she is fed up" },
];
const STYLES = [{ id: "whimsical", label: "Whimsical illustration", text: "a whimsical register" }];
const PROVIDERS = [{ id: "fal", hasKey: true, models: [{ id: "fal/nb2", label: "Nano Banana 2", usd: 0.08, known: true }] }];
const CONDITIONS = [
  { name: "lateNight", class: "ambient", slots: ["idle", "working", "bored"], label: "it is late" },
  { name: "weekend", class: "ambient", slots: ["idle", "working", "bored"], label: "it is the weekend" },
];

async function openWizard(page, patch = {}) {
  await page.goto("/?nointro");
  await page.evaluate(({ SLOTS, STYLES, PROVIDERS, CONDITIONS, patch }) => {
    const g = window.__packgen;
    Object.assign(g, {
      open: true, run: null, step: "base",
      slotDefaults: SLOTS, styles: STYLES, providers: PROVIDERS, conditions: CONDITIONS,
      styleID: "whimsical", providerID: "fal", model: "fal/nb2",
      name: "", base: null, cost: null, error: "", busy: false,
      picks: Object.fromEntries(SLOTS.map((s) => [s.slot, { on: true, staging: s.staging }])),
    });
    Object.assign(g, patch);
  }, { SLOTS, STYLES, PROVIDERS, CONDITIONS, patch });
  await expect(page.getByTestId("packgen-panel")).toBeVisible();
}

const item = (slot, over = {}) => ({ slot, when: "", status: "done", preview: `id-${slot}`, error: "", ...over });

test("the panel is NON-MODAL — the deck behind it stays usable", async ({ page }) => {
  await openWizard(page);

  // No modal semantics: a run takes minutes, so trapping the user in the
  // wizard is not an option.
  await expect(page.getByTestId("packgen-panel")).not.toHaveAttribute("aria-modal");

  // The titlebar nav is still reachable and still works while the panel is up.
  // This is the assertion that actually bites: a fixed backdrop with inset:0
  // would cover the titlebar and make this click fail.
  const settings = page.getByTestId("open-settings");
  await expect(settings).toBeVisible();
  await settings.click();
  await expect(page.getByTestId("drawer-settings")).toBeVisible();
  // ...and the panel survived that, because it is not tied to the drawer.
  await expect(page.getByTestId("packgen-panel")).toBeVisible();
});

test("closing the panel does not end the run", async ({ page }) => {
  await openWizard(page, {
    run: { id: "r1", status: "running", total: 3, done: 1, billed: 1, saved: false, items: [item("idle")] },
  });
  await expect(page.getByTestId("packgen-grid")).toBeVisible();

  await page.getByTestId("packgen-close").click();
  await expect(page.getByTestId("packgen-panel")).toHaveCount(0);

  // The run is untouched — only the panel's visibility changed.
  const status = await page.evaluate(() => window.__packgen.run.status);
  expect(status).toBe("running");
});

// The central rule: the phase is derived from the run. A wizard reopened
// mid-run must land on the grid, not on step 1 with a Generate button over a
// run that is already spending money.
test("reopening mid-run shows the grid, not the form", async ({ page }) => {
  await openWizard(page, {
    step: "base", // stale: the user was on step 1 when they started
    run: {
      id: "r1", status: "running", total: 3, done: 1, billed: 1, saved: false,
      items: [item("idle"), item("working", { status: "running", preview: "" }), item("bored", { status: "pending", preview: "" })],
    },
  });

  await expect(page.getByTestId("packgen-grid")).toBeVisible();
  await expect(page.getByTestId("packgen-name")).toHaveCount(0);
  await expect(page.getByTestId("packgen-start")).toHaveCount(0);
  await expect(page.getByTestId("packgen-cancel")).toBeVisible();
  // The rail marks where we are.
  await expect(page.getByTestId("packgen-step-generate")).toHaveClass(/now/);
});

test("a finished run with unsaved art lands on review with both decisions", async ({ page }) => {
  await openWizard(page, {
    step: "base",
    run: {
      id: "r1", status: "done", total: 2, done: 2, billed: 2, saved: false,
      items: [item("idle"), item("working")],
    },
  });

  await expect(page.getByTestId("packgen-save")).toBeVisible();
  await expect(page.getByTestId("packgen-discard")).toBeVisible();
  await expect(page.getByTestId("packgen-cancel")).toHaveCount(0);
  await expect(page.getByTestId("packgen-step-review")).toHaveClass(/now/);
});

test("cards show previews through the pack media route", async ({ page }) => {
  await openWizard(page, {
    run: {
      id: "r1", status: "done", total: 2, done: 1, billed: 2, saved: false,
      items: [item("idle"), item("working", { status: "failed", preview: "", error: "the provider flagged this image" })],
    },
  });

  const good = page.getByTestId("packgen-card-idle");
  await expect(good.locator("img")).toHaveAttribute("src", "/media/pack/id-idle");

  // A failed card must say WHY rather than showing an empty box.
  const bad = page.getByTestId("packgen-card-working");
  await expect(bad.locator("img")).toHaveCount(0);
  await expect(bad).toContainText("flagged");
});

test("the run line reports what was billed, not just what succeeded", async ({ page }) => {
  await openWizard(page, {
    run: {
      id: "r1", status: "done", total: 3, done: 2, billed: 3, saved: false,
      items: [item("idle"), item("working"), item("bored", { status: "failed", preview: "", error: "black frame" })],
    },
  });
  const line = page.getByTestId("packgen-runline");
  await expect(line).toContainText("2");
  await expect(line).toContainText("3 billed");
});

// A disabled button with no explanation is the most common way a wizard
// dead-ends. Every blocked state must say what to do about it.
test("the Generate button explains itself when unavailable", async ({ page }) => {
  await openWizard(page, { step: "slots", name: "", base: null });

  await expect(page.getByTestId("packgen-start")).toBeDisabled();
  await expect(page.getByTestId("packgen-blocker")).toContainText(/name/i);

  await page.evaluate(() => { window.__packgen.name = "Wafa"; });
  await expect(page.getByTestId("packgen-blocker")).toContainText(/portrait/i);

  await page.evaluate(() => { window.__packgen.base = { path: "/tmp/a.png", w: 40, h: 60 }; });
  await expect(page.getByTestId("packgen-blocker")).toHaveCount(0);
  await expect(page.getByTestId("packgen-start")).toBeEnabled();
});

test("two ambient conditions on one slot are refused before any spend", async ({ page }) => {
  await openWizard(page, {
    step: "slots", name: "Wafa", base: { path: "/tmp/a.png", w: 40, h: 60 },
  });
  await expect(page.getByTestId("packgen-start")).toBeEnabled();

  await page.getByTestId("packgen-cond-bored-lateNight").locator("input").check();
  await page.getByTestId("packgen-cond-bored-weekend").locator("input").check();

  await expect(page.getByTestId("packgen-blocker")).toContainText(/both be true at once/i);
  await expect(page.getByTestId("packgen-start")).toBeDisabled();
});

test("idle cannot be unticked, because a pack without it warns on every load", async ({ page }) => {
  await openWizard(page, { step: "slots" });
  await expect(page.getByTestId("packgen-slot-idle-on")).toBeDisabled();
  await expect(page.getByTestId("packgen-slot-working-on")).toBeEnabled();
});

test("a landscape base warns but does not block", async ({ page }) => {
  await openWizard(page, {
    name: "Wafa",
    base: { path: "/tmp/wide.png", w: 80, h: 20, warning: "This is a landscape image." },
  });
  await expect(page.getByTestId("packgen-base-warning")).toContainText(/landscape/i);
  await page.evaluate(() => { window.__packgen.step = "slots"; });
  await expect(page.getByTestId("packgen-start")).toBeEnabled();
});

test("an unknown price shows a count rather than a figure", async ({ page }) => {
  await openWizard(page, {
    step: "slots", cost: { usd: 0, known: false, note: "prices as of 2026-08-30" },
  });
  const summary = page.getByTestId("packgen-summary");
  await expect(summary).toContainText("3 images");
  await expect(summary).toContainText(/unknown/i);
  await expect(summary).not.toContainText("$0.00");
});

test("a known price is shown with its date", async ({ page }) => {
  await openWizard(page, {
    step: "slots", cost: { usd: 0.24, known: true, note: "prices as of 2026-08-30" },
  });
  const summary = page.getByTestId("packgen-summary");
  await expect(summary).toContainText("$0.24");
  await expect(summary).toContainText("prices as of");
});

test("the panel mounts with no console errors", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push("" + e));
  await openWizard(page, { step: "slots" });
  await page.evaluate(() => { window.__packgen.step = "base"; });
  await expect(page.getByTestId("packgen-name")).toBeVisible();
  expect(errors).toEqual([]);
});

// --- the saved-pack browser -----------------------------------------------

const SAVED = {
  name: "Nova", path: "/packs/nova", hasBase: true, warnings: [],
  variants: [
    { slot: "idle", when: "", file: "idle.png", preview: "id-idle", focusX: 0.5, focusY: 0,
      prompt: "the idle prompt", model: "fal/nb2", seed: "42", generatedAt: "2026-08-30T12:00:00Z" },
    { slot: "working", when: "", file: "working.png", preview: "id-working", focusX: 0.5, focusY: 0,
      prompt: "the working prompt", model: "fal/nb2", seed: "43", generatedAt: "2026-08-30T12:01:00Z" },
    { slot: "bored", when: "lateNight", file: "bored-latenight.png", preview: "id-night",
      focusX: 0.7, focusY: 0.1, prompt: "the night prompt", model: "fal/nb2", seed: "44",
      generatedAt: "2026-08-30T12:02:00Z" },
  ],
};

async function openBrowse(page, browse = SAVED) {
  await openWizard(page, { browse });
  await page.getByTestId("packgen-tab-browse").click();
  await expect(page.getByTestId("packgen-browse")).toBeVisible();
}

test("the browser lists every saved scene with its generation date", async ({ page }) => {
  await openBrowse(page);
  await expect(page.getByTestId("packgen-saved-idle")).toBeVisible();
  await expect(page.getByTestId("packgen-saved-working")).toBeVisible();
  // A conditioned variant must be listed distinctly from its base sibling, or
  // regenerating one would silently target the other.
  await expect(page.getByTestId("packgen-saved-bored-lateNight")).toBeVisible();
  await expect(page.getByTestId("packgen-tab-browse")).toContainText("3");
});

test("a scene's detail shows the prompt that produced it", async ({ page }) => {
  await openBrowse(page);
  await page.getByTestId("packgen-saved-working").getByRole("button").first().click();
  await expect(page.getByTestId("packgen-prompt-working")).toHaveValue("the working prompt");
  await expect(page.getByTestId("packgen-regen-working")).toBeEnabled();
});

// The whole point of the feature: one scene, one image. Asserted on the
// payload that actually crosses the binding, by stubbing the Wails bridge the
// generated wrapper calls — a test that only checked a function exists would
// pass no matter what it sent.
test("regenerating one scene sends only that slot, with the edited prompt", async ({ page }) => {
  await openBrowse(page);
  await page.evaluate(() => {
    window.__sent = [];
    window.go = { main: { App: {
      StartPackGen: (req) => {
        window.__sent.push(req);
        return Promise.resolve({ id: "r9", status: "running", total: 1, done: 0,
                                 billed: 0, saved: false, items: [] });
      },
    } } };
  });

  await page.getByTestId("packgen-saved-working").getByRole("button").first().click();
  await page.getByTestId("packgen-prompt-working").fill("she is scowling at a compiler");
  await page.getByTestId("packgen-regen-working").click();

  const sent = await page.evaluate(() => window.__sent);
  expect(sent).toHaveLength(1);
  expect(sent[0].regenerate, "must run in regenerate mode, not build a new pack").toBe(true);
  expect(sent[0].slots).toHaveLength(1);
  expect(sent[0].slots[0].slot).toBe("working");
  // The prompt shown is the prompt sent — no silent recomposition, so what the
  // user edits is exactly what they pay for.
  expect(sent[0].slots[0].prompt).toBe("she is scowling at a compiler");
  // No base path: Go takes the portrait from inside the saved pack.
  expect(sent[0].basePath).toBe("");
  expect(sent[0].name).toBe("Nova");
});

test("the regenerate button shows the per-image price, not the pack's", async ({ page }) => {
  await openBrowse(page);
  await page.getByTestId("packgen-saved-working").getByRole("button").first().click();
  await expect(page.getByTestId("packgen-regen-working")).toContainText("$0.08");
});

// A pack with no stored base cannot be regenerated from. Offering a button that
// fails after it is pressed is worse than not offering one.
test("a pack with no stored portrait cannot regenerate", async ({ page }) => {
  await openBrowse(page, { ...SAVED, hasBase: false });
  await page.getByTestId("packgen-saved-idle").getByRole("button").first().click();
  await expect(page.getByTestId("packgen-regen-idle")).toBeDisabled();
  await expect(page.getByTestId("packgen-browse")).toContainText(/no stored portrait/i);
});

test("the crop sliders start from the saved values", async ({ page }) => {
  await openBrowse(page);
  await page.getByTestId("packgen-saved-bored-lateNight").getByRole("button").first().click();
  await expect(page.getByTestId("packgen-focusx-bored")).toHaveValue("0.7");
  await expect(page.getByTestId("packgen-focusy-bored")).toHaveValue("0.1");
});

// The thumbnail must show the SAVED crop, or the slider would be adjusting
// something the user cannot see the effect of.
test("thumbnails honour each variant's crop", async ({ page }) => {
  await openBrowse(page);
  const img = page.getByTestId("packgen-saved-bored-lateNight").locator("img");
  await expect(img).toHaveAttribute("style", /object-position:\s*70% 10%/);
});

test("the browser reports an empty state rather than failing", async ({ page }) => {
  await openBrowse(page, null);
  await expect(page.getByTestId("packgen-browse")).toContainText(/no pack saved/i);
});

// Switching tabs must not disturb a run: it is a different view of different
// state, not a step in the wizard.
test("the browser is reachable while a run is generating", async ({ page }) => {
  await openWizard(page, {
    browse: SAVED,
    run: { id: "r1", status: "running", total: 2, done: 1, billed: 1, saved: false,
           items: [item("idle")] },
  });
  await expect(page.getByTestId("packgen-grid")).toBeVisible();
  await page.getByTestId("packgen-tab-browse").click();
  await expect(page.getByTestId("packgen-browse")).toBeVisible();

  await page.getByTestId("packgen-tab-create").click();
  await expect(page.getByTestId("packgen-grid")).toBeVisible();
  expect(await page.evaluate(() => window.__packgen.run.status)).toBe("running");
});

// Regeneration must be unavailable while a run is in flight — the run slot is
// single, and Go would refuse it anyway.
test("regenerate is disabled while a run is already generating", async ({ page }) => {
  await openWizard(page, {
    browse: SAVED,
    run: { id: "r1", status: "running", total: 1, done: 0, billed: 0, saved: false, items: [] },
  });
  await page.getByTestId("packgen-tab-browse").click();
  await page.getByTestId("packgen-saved-idle").getByRole("button").first().click();
  await expect(page.getByTestId("packgen-regen-idle")).toBeDisabled();
});

// --- the pack library -----------------------------------------------------

const LIBRARY = [
  { name: "Another", path: "/packs/another", folder: "another", scenes: 1,
    thumb: "id-another", hasBase: false, active: false, warnings: 0 },
  { name: "Nova", path: "/packs/nova", folder: "nova", scenes: 10,
    thumb: "id-idle", hasBase: true, active: true, warnings: 0 },
];

// With several packs, "which one am I looking at" has to be answerable before
// anything else on the tab makes sense.
test("the library lists every pack and marks the active one", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => { window.__packgen.library = lib; }, LIBRARY);

  const shelf = page.getByTestId("packgen-library");
  await expect(shelf).toBeVisible();
  await expect(page.getByTestId("packgen-lib-nova")).toContainText("Nova");
  await expect(page.getByTestId("packgen-lib-nova")).toContainText("10 scenes");
  await expect(page.getByTestId("packgen-lib-nova")).toContainText("active");
  await expect(page.getByTestId("packgen-lib-another")).toContainText("1 scene");
  await expect(page.getByTestId("packgen-lib-another")).not.toContainText("active");
});

test("each pack shows its own thumbnail", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => { window.__packgen.library = lib; }, LIBRARY);
  await expect(page.getByTestId("packgen-lib-another").locator("img"))
    .toHaveAttribute("src", "/media/pack/id-another");
});

test("clicking an inactive pack switches to it", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__selected = [];
    window.go = { main: { App: {
      SelectCompanionPack: (p) => { window.__selected.push(p); return Promise.resolve("Another"); },
      BrowseCompanionPack: () => Promise.resolve(null),
      ListCompanionPacks: () => Promise.resolve([]),
      LoadCompanionPack: () => Promise.resolve(null),
    } } };
  }, LIBRARY);

  await page.getByTestId("packgen-lib-another").click();
  expect(await page.evaluate(() => window.__selected)).toEqual(["/packs/another"]);
});

// Clicking the pack you are already on should do nothing rather than churn
// through a reload of everything.
test("clicking the active pack does not re-select it", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__selected = [];
    window.go = { main: { App: {
      SelectCompanionPack: (p) => { window.__selected.push(p); return Promise.resolve("Nova"); },
    } } };
  }, LIBRARY);

  await page.getByTestId("packgen-lib-nova").click();
  expect(await page.evaluate(() => window.__selected)).toEqual([]);
});

// A pack whose manifest produced warnings is worth flagging in the shelf: it
// is the only place the user can see that one of their packs is degraded.
test("a pack with warnings is flagged in the library", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib.map((p) =>
      p.folder === "another" ? { ...p, warnings: 3 } : p);
  }, LIBRARY);
  await expect(page.getByTestId("packgen-lib-another").locator(".warnbadge")).toBeVisible();
  await expect(page.getByTestId("packgen-lib-nova").locator(".warnbadge")).toHaveCount(0);
});

// A pack with no usable idle image must show a placeholder rather than a
// broken picture.
test("a pack with no thumbnail still renders", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib.map((p) =>
      p.folder === "another" ? { ...p, thumb: "" } : p);
  }, LIBRARY);
  const card = page.getByTestId("packgen-lib-another");
  await expect(card).toBeVisible();
  await expect(card.locator("img")).toHaveCount(0);
  await expect(card).toContainText("Another");
});

// --- backup and restore ---------------------------------------------------

test("export and import are offered on the browse tab", async ({ page }) => {
  await openBrowse(page, SAVED);
  await expect(page.getByTestId("packgen-import")).toBeEnabled();
  await expect(page.getByTestId("packgen-export")).toBeEnabled();
});

// Import is always available — it is how you get your FIRST pack back after a
// reinstall, when there is nothing to browse.
test("import works with no pack configured, export does not", async ({ page }) => {
  await openBrowse(page, null);
  await expect(page.getByTestId("packgen-import")).toBeEnabled();
  await expect(page.getByTestId("packgen-export")).toBeDisabled();
});

// --- the model picker -----------------------------------------------------

const MULTI = [{
  id: "fal", hasKey: true, models: [
    { id: "fal-ai/nano-banana-2/edit", label: "Nano Banana 2", usd: 0.08, known: true,
      refusal: "unknown", note: "Best identity and framing in testing. Slower." },
    { id: "fal-ai/flux-2/klein/4b/edit", label: "FLUX.2 klein 4B", usd: 0.01, known: true,
      refusal: "black_image",
      note: "8x cheaper and ~5x faster. Around 1 in 6 images comes back rejected by fal's safety filter — and those are still charged." },
  ],
}];

test("the picker lists every model with its price", async ({ page }) => {
  await openWizard(page, { providers: MULTI, model: "fal-ai/nano-banana-2/edit" });
  const opts = page.getByTestId("packgen-model").locator("option");
  await expect(opts).toHaveCount(2);
  await expect(opts.nth(0)).toContainText("$0.08");
  await expect(opts.nth(1)).toContainText("$0.01");
});

// The cheap model is billed for its own refusals. The user should learn that
// before the run, not from the bill.
test("a model that refuses at the user's expense says so up front", async ({ page }) => {
  await openWizard(page, { providers: MULTI, model: "fal-ai/nano-banana-2/edit" });
  await expect(page.getByTestId("packgen-model-note")).not.toHaveClass(/warn/);

  await page.getByTestId("packgen-model").selectOption("fal-ai/flux-2/klein/4b/edit");
  const note = page.getByTestId("packgen-model-note");
  await expect(note).toContainText(/still charged/i);
  await expect(note).toHaveClass(/warn/);
});

test("the estimate follows the selected model", async ({ page }) => {
  await openWizard(page, {
    providers: MULTI, model: "fal-ai/flux-2/klein/4b/edit", step: "slots",
    cost: { usd: 0.03, known: true, note: "prices as of 2026-08-31" },
  });
  await expect(page.getByTestId("packgen-summary")).toContainText("$0.03");
});

// --- deleting a pack ------------------------------------------------------

// Deleting art is the one irreversible thing in this panel, so it must never
// happen on a single click.
test("delete asks first, and cancelling changes nothing", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__deleted = [];
    window.go = { main: { App: {
      DeleteCompanionPack: (p) => { window.__deleted.push(p); return Promise.resolve(); },
    } } };
  }, LIBRARY);

  await page.getByTestId("packgen-del-another").click();
  await expect(page.getByTestId("packgen-delete-confirm")).toBeVisible();
  // Nothing has happened yet.
  expect(await page.evaluate(() => window.__deleted)).toEqual([]);

  await page.getByTestId("packgen-delete-cancel").click();
  await expect(page.getByTestId("packgen-delete-confirm")).toHaveCount(0);
  expect(await page.evaluate(() => window.__deleted)).toEqual([]);
});

test("confirming deletes exactly the pack that was named", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__deleted = [];
    window.go = { main: { App: {
      DeleteCompanionPack: (p) => { window.__deleted.push(p); return Promise.resolve(); },
      BrowseCompanionPack: () => Promise.resolve(null),
      ListCompanionPacks: () => Promise.resolve([]),
      LoadCompanionPack: () => Promise.resolve(null),
    } } };
  }, LIBRARY);

  await page.getByTestId("packgen-del-another").click();
  await expect(page.getByTestId("packgen-delete-confirm")).toContainText("Another");
  await page.getByTestId("packgen-delete-go").click();
  expect(await page.evaluate(() => window.__deleted)).toEqual(["/packs/another"]);
});

// The dialog must say what is being destroyed, in units the user recognises.
test("the confirmation names the pack, its size and its path", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => { window.__packgen.library = lib; }, LIBRARY);
  await page.getByTestId("packgen-del-nova").click();

  const dlg = page.getByTestId("packgen-delete-confirm");
  await expect(dlg).toContainText("Nova");
  await expect(dlg).toContainText("10 image");
  await expect(dlg).toContainText("/packs/nova");
  await expect(dlg).toContainText(/cannot be undone/i);
  // Deleting the ACTIVE pack leaves the companion with none — worth saying.
  await expect(dlg).toContainText(/currently in use/i);
  // And it points at the recoverable alternative.
  await expect(dlg).toContainText(/export/i);
});

test("escape dismisses the confirmation without deleting", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__deleted = [];
    window.go = { main: { App: { DeleteCompanionPack: (p) => { window.__deleted.push(p); return Promise.resolve(); } } } };
  }, LIBRARY);

  await page.getByTestId("packgen-del-another").click();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("packgen-delete-confirm")).toHaveCount(0);
  expect(await page.evaluate(() => window.__deleted)).toEqual([]);
});

// The delete button sits inside the card, so a click must not also select the
// pack it is about to remove.
test("clicking delete does not switch to that pack", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.library = lib;
    window.__selected = [];
    window.go = { main: { App: {
      SelectCompanionPack: (p) => { window.__selected.push(p); return Promise.resolve("x"); },
    } } };
  }, LIBRARY);

  await page.getByTestId("packgen-del-another").click();
  expect(await page.evaluate(() => window.__selected)).toEqual([]);
});

// --- pack addressing ------------------------------------------------------

// The folder is slug(name) only for packs this app generated. Addressing by
// name regenerated a DIFFERENT pack when the two diverged, which is what an
// imported pack always does.
test("regenerate addresses the pack by path, not by display name", async ({ page }) => {
  await openBrowse(page, { ...SAVED, name: "Nova", path: "/packs/nova-2" });
  await page.evaluate(() => {
    window.__sent = [];
    window.go = { main: { App: {
      StartPackGen: (req) => { window.__sent.push(req);
        return Promise.resolve({ id: "r", status: "running", total: 1, done: 0, billed: 0, saved: false, items: [] }); },
    } } };
  });
  await page.getByTestId("packgen-saved-working").getByRole("button").first().click();
  await page.getByTestId("packgen-regen-working").click();

  const sent = await page.evaluate(() => window.__sent);
  expect(sent[0].packPath).toBe("/packs/nova-2");
  expect(sent[0].regenerate).toBe(true);
});

// Re-rolling one scene with the Create tab's model puts a stranger into a set
// whose entire premise is that it is one character.
test("regenerate uses the variant's own model", async ({ page }) => {
  await openBrowse(page, SAVED);
  await page.evaluate((lib) => {
    window.__packgen.providers = lib;
    window.__packgen.model = "fal-ai/flux-2/klein/4b/edit"; // a DIFFERENT model selected
    window.__sent = [];
    window.go = { main: { App: {
      StartPackGen: (req) => { window.__sent.push(req);
        return Promise.resolve({ id: "r", status: "running", total: 1, done: 0, billed: 0, saved: false, items: [] }); },
    } } };
  }, MULTI);
  await page.getByTestId("packgen-saved-working").getByRole("button").first().click();
  await page.getByTestId("packgen-regen-working").click();

  const sent = await page.evaluate(() => window.__sent);
  expect(sent[0].model).toBe("fal/nb2"); // the variant's recorded model
});

// --- not silently replacing a pack ----------------------------------------

test("a name collision becomes a decision, not a dead end", async ({ page }) => {
  await openWizard(page, {
    step: "slots", name: "Nova", base: { path: "/tmp/a.png", w: 40, h: 60 },
  });
  await page.evaluate(() => {
    window.__calls = [];
    window.go = { main: { App: {
      StartPackGen: (req) => {
        window.__calls.push(req);
        if (!req.overwrite) {
          return Promise.reject(new Error(
            'a pack called "nova" already exists (/packs/nova). Saving would replace it and its images cannot be recovered'));
        }
        return Promise.resolve({ id: "r", status: "running", total: 1, done: 0, billed: 0, saved: false, items: [] });
      },
    } } };
  });

  await page.getByTestId("packgen-start").click();
  const dlg = page.getByTestId("packgen-overwrite-confirm");
  await expect(dlg).toBeVisible();
  await expect(dlg).toContainText("already exists");
  // It must NOT read as a generic failure.
  await expect(page.getByTestId("packgen-error")).toHaveCount(0);

  await page.getByTestId("packgen-overwrite-cancel").click();
  await expect(dlg).toHaveCount(0);
  expect(await page.evaluate(() => window.__calls.length)).toBe(1);

  await page.getByTestId("packgen-start").click();
  await page.getByTestId("packgen-overwrite-go").click();
  const calls = await page.evaluate(() => window.__calls);
  expect(calls[calls.length - 1].overwrite).toBe(true);
});

// A configured pack whose folder was moved or deleted outside the app. Go now
// says so explicitly; before, it returned a valid struct with a NIL variants
// slice and the tab threw on `.length`.
test("a pack whose folder is gone says so instead of breaking", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push("" + e));
  await openBrowse(page, {
    name: "Nova", path: "/packs/nova", hasBase: false,
    variants: [], warnings: [], missing: true,
  });
  await expect(page.getByTestId("packgen-missing")).toContainText(/no longer/i);
  await expect(page.getByTestId("packgen-missing")).toContainText("/packs/nova");
  expect(errors).toEqual([]);
});

// The shape Go actually produced when it went wrong.
test("a null variants list does not throw", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push("" + e));
  await openBrowse(page, {
    name: "Nova", path: "/packs/nova", hasBase: true,
    variants: null, warnings: null, missing: false,
  });
  await expect(page.getByTestId("packgen-browse")).toBeVisible();
  expect(errors).toEqual([]);
});
