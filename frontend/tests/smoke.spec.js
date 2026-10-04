import { test, expect } from "@playwright/test";

// Wails bindings (window.go) don't exist in a plain browser; binding calls
// reject but the shell MUST still mount (the B1 blank-window bug class).
test("app mounts the deck shell", async ({ page }) => {
  await page.goto("/?nointro");
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("wordmark")).toHaveText("COMMANDER");
  await expect(page.getByTestId("launch-button")).toBeVisible();
  await expect(page.getByTestId("folder-input")).toBeVisible();
  await expect(page.getByTestId("empty-state")).toBeVisible();
  // With no dock snippet the band must collapse entirely, so the public build
  // (which passes none) lays out exactly as it did before this plan.
  const dockH = await page.getByTestId("sidebar-dock").evaluate((el) => el.getBoundingClientRect().height);
  expect(dockH).toBe(0);
});

test("providers drawer opens and closes on ESC", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("drawer-providers")).not.toBeVisible();
});

test("models drawer opens with add form", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-models").click();
  await expect(page.getByTestId("drawer-models")).toBeVisible();
  await expect(page.getByTestId("add-model-id")).toBeVisible();
  await expect(page.getByTestId("add-model-submit")).toBeVisible();
});

test("models drawer shows anthropic section", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-models").click();
  await expect(page.getByTestId("drawer-models")).toBeVisible();
  await expect(page.getByTestId("models-section-anthropic")).toBeVisible();
});

test("launch without folder shows inline error, not a toast", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("launch-button").click();
  await expect(page.getByTestId("launch-error")).toBeVisible();
});

test("accent choice persists across reload", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("accent-hue").fill("200");
  await page.reload();
  await page.goto("/?nointro");
  const accent = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue("--accent"),
  );
  if (!accent.includes("200")) throw new Error(`accent not persisted: ${accent}`);
});

test("command palette opens on ⌘K and closes on Escape", async ({ page }) => {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  await expect(page.getByTestId("palette-input")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("palette")).not.toBeVisible();
});

test("⌘K opens the palette over an open drawer instead of the drawer stealing focus back", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  await expect(page.getByTestId("palette-input")).toBeFocused();
});

test("boot intro suppressed once played flag is set", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => localStorage.setItem("commander.introPlayed.v1", "1"));
  await page.goto("/"); // no ?nointro: flag alone must suppress it
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("boot-intro")).not.toBeVisible();
});

test("palette fuzzy-filters and opens settings drawer on Enter", async ({ page }) => {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await page.getByTestId("palette-input").fill("set");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("drawer-settings")).toBeVisible();
});

test("font size pref persists and hotkey bumps it", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("font-size").fill("16");
  await page.keyboard.press("Escape");
  await page.keyboard.press("Meta+Equal");
  await page.reload();
  await page.goto("/?nointro");
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v2")).fontSize);
  if (stored !== 17) throw new Error(`expected 17, got ${stored}`);
});

test("sidebar divider drag persists width", async ({ page }) => {
  await page.goto("/?nointro");
  const d = page.getByTestId("sidebar-divider");
  const box = await d.boundingBox();
  await page.mouse.move(box.x + 2, box.y + 200);
  await page.mouse.down();
  await page.mouse.move(380, box.y + 200);
  await page.mouse.up();
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v2")).sidebarW);
  if (stored < 360 || stored > 400) throw new Error(`expected ~380, got ${stored}`);
});

test("rc auto toggle persists", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await page.getByTestId("rc-auto-toggle").check();
  await page.reload();
  await page.goto("/?nointro");
  const stored = await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v2")).rcAutoEnable);
  if (stored !== true) throw new Error("rcAutoEnable not persisted");
});

test("usage drawer opens and shows fetch error in plain browser", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-usage").click();
  await expect(page.getByTestId("drawer-usage")).toBeVisible();
});

test("the six nav buttons live in the titlebar and still open their drawers", async ({ page }) => {
  await page.goto("/?nointro");
  const bar = page.getByTestId("titlebar");
  for (const id of ["open-history", "open-about", "open-providers",
                    "open-models", "open-usage", "open-settings"]) {
    await expect(bar.getByTestId(id)).toBeVisible();
  }
  // The sidebar must no longer carry them: a duplicate id would make every
  // existing smoke's getByTestId ambiguous and fail with a strict-mode violation.
  await expect(page.getByTestId("sidebar").getByTestId("open-settings")).toHaveCount(0);

  // Wails treats the titlebar as a drag region; a control inside one does not
  // receive clicks unless it opts out. Assert the opt-out is really computed,
  // not just written in a stylesheet that never matched.
  const draggable = await page.getByTestId("open-settings").evaluate((el) =>
    getComputedStyle(el).getPropertyValue("--wails-draggable").trim());
  expect(draggable).toBe("no-drag");

  await page.getByTestId("open-providers").click();
  await expect(page.getByTestId("drawer-providers")).toBeVisible();
});

test("the session list scrolls independently at both sidebar width extremes", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = Array.from({ length: 30 }, (_, i) => ({
      windowID: "w" + i, name: "project-" + i,
    }));
  });
  const list = page.getByTestId("session-list");
  for (const w of [240, 480]) {
    await page.evaluate((px) => {
      document.documentElement.style.setProperty("--sidebar-w", px + "px");
    }, w);
    const m = await list.evaluate((el) => ({
      scrollH: el.scrollHeight, clientH: el.clientHeight,
    }));
    expect(m.scrollH, `list must overflow at ${w}px`).toBeGreaterThan(m.clientH);
    // It must actually REACH the end: a mis-set min-height clips the tail with
    // no scrollbar, which looks fine in a screenshot and loses sessions.
    await list.evaluate((el) => { el.scrollTop = el.scrollHeight; });
    await expect(page.getByTestId("session-card").last()).toBeInViewport();
    // The aside itself must NOT be the scroller, or the dock scrolls away.
    const asideScrolls = await page.getByTestId("sidebar").evaluate((el) =>
      el.scrollHeight > el.clientHeight + 1);
    expect(asideScrolls, `the aside itself scrolled at ${w}px`).toBe(false);
  }
});

test("a latched app:error tints the selected session's card, and only that one", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [
      { windowID: "w1", name: "one" },
      { windowID: "w2", name: "two" },
    ];
    window.__app.sessionKey = "w1:1";
  });
  const cards = page.getByTestId("session-card");
  await expect(cards).toHaveCount(2);
  await expect(cards.first()).not.toHaveClass(/errored/);

  await page.evaluate(() => { window.__app.errorMs = Date.now(); });
  await expect(cards.first()).toHaveClass(/errored/);
  await expect(cards.nth(1)).not.toHaveClass(/errored/); // never smears to the other card

  // Selecting the other session moves the latch's attribution with it — the
  // same "attributed to whoever is selected" property the error latch in the
  // shared store provides, exercised here through the card that consumes it.
  await page.evaluate(() => { window.__app.sessionKey = "w2:2"; });
  await expect(cards.first()).not.toHaveClass(/errored/);
  await expect(cards.nth(1)).toHaveClass(/errored/);
});

test("an expired app:error clears once its TTL passes and the 5s poll ticks", async ({ page }) => {
  // Svelte 5 reactivity only re-evaluates isErrored() when one of its tracked
  // reads (app.errorMs, app.sessionKey) changes — Date.now() itself is not a
  // tracked source. Without refresh() explicitly zeroing app.errorMs once it
  // is stale, a card that went errored would stay tinted forever if nothing
  // else happened to touch either field (a reactive-staleness bug class this
  // project has already hit once, where a plain function call was read in a
  // template and therefore never re-evaluated). Using
  // the clock instead of a manual page.evaluate reset proves the poll itself
  // does the clearing, not the test.
  await page.clock.install({ time: new Date() });
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.sessionKey = "w1:1";
    window.__app.errorMs = Date.now();
  });
  await expect(page.getByTestId("session-card")).toHaveClass(/errored/);
  await page.clock.fastForward(35000); // past ERROR_TTL_MS (30s) + a poll tick
  await expect(page.getByTestId("session-card")).not.toHaveClass(/errored/);
});

// The card's telemetry reads context fullness against the MODEL'S OWN window.
// Both the number and the meter used to divide by a hardcoded 200k, so a 1M
// model sat near-full and permanently red at 16% used — a meter that said
// nothing. Asserting the rendered text and the meter width together, because
// the two were separate hardcodings and fixing one would not fix the other.
test("a session card measures context against the model's own window", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.stats = { w1: {
      contextTokens: 160_000, contextWindow: 1_000_000, band: "green",
      turns: 3, model: "claude-opus-5-5", provider: "anthropic",
      uptimeSeconds: 60, status: "active", remoteControl: false, cwd: "/tmp/x",
    } };
  });
  const card = page.getByTestId("session-card");
  await expect(card).toContainText("160k / 1M");
  // No dollar figure survives anywhere on the card.
  await expect(card).not.toContainText("$");
  // 160k of 1M is 16%, not the 80% a 200k divisor would have produced.
  const width = await card.locator(".fill").evaluate((el) => el.style.width);
  expect(parseFloat(width)).toBeGreaterThan(14);
  expect(parseFloat(width)).toBeLessThan(18);
});

// A model that reports no window (routed providers, hand-added entries) must
// still render a scale rather than dividing by zero.
test("a session card falls back to a usable window when the model reports none", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.stats = { w1: {
      contextTokens: 50_000, contextWindow: 0, band: "green",
      turns: 1, model: "some-routed-model", provider: "zen",
      uptimeSeconds: 60, status: "active", remoteControl: false, cwd: "/tmp/x",
    } };
  });
  await expect(page.getByTestId("session-card")).toContainText("50k / 200k");
});

const SWAP_STAT = { provider: "anthropic", model: "opus", contextTokens: 1, contextWindow: 200000, turns: 1, uptimeSeconds: 60, band: "ok", cwd: "/x/one" };

test("the card swap control groups models with optgroups", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate((stat) => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.stats = { w1: stat };
    window.__app.models = [
      { id: "a", label: "Anthropic · Opus 5.5", provider: "anthropic", routed: false, ready: true },
      { id: "o", label: "Ollama · glm", provider: "ollama-cloud", routed: true, ready: true },
    ];
  }, SWAP_STAT);
  const swap = page.getByTestId("swap-select");
  await expect(swap.locator("optgroup")).toHaveCount(2);
  await expect(swap.locator("optgroup").nth(0)).toHaveAttribute("label", "Anthropic");
  await expect(swap.locator("optgroup").nth(1)).toHaveAttribute("label", "Ollama Cloud");
  // Prefix stripped, since the optgroup label supplies it; value stays the id.
  await expect(swap.locator("optgroup").nth(0).locator("option")).toHaveText("Opus 5.5");
  await expect(swap.locator("optgroup").nth(1).locator("option")).toHaveText("glm");
  await expect(swap.locator("optgroup").nth(1).locator("option")).toHaveAttribute("value", "o");
});

test("a single-provider catalog gives the card swap no optgroup and full labels", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate((stat) => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.stats = { w1: stat };
    window.__app.models = [
      { id: "a", label: "Anthropic · Opus 5.5", provider: "anthropic", routed: false, ready: true },
      { id: "b", label: "Anthropic · Sonnet 5.5", provider: "anthropic", routed: false, ready: true },
    ];
  }, SWAP_STAT);
  const swap = page.getByTestId("swap-select");
  await expect(swap.locator("option")).toHaveCount(3); // swap… + 2
  await expect(swap.locator("optgroup")).toHaveCount(0);
  await expect(swap.locator("option").nth(1)).toHaveText("Anthropic · Opus 5.5");
});

test("palette swap rows carry the provider as their hint and keep full labels", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.sessionKey = "w1:1";
    // Labels deliberately do not name their provider, so only the hint can.
    window.__app.models = [
      { id: "a", label: "Opus 5.5", provider: "anthropic", routed: false, ready: true },
      { id: "o", label: "GLM Flash", provider: "ollama-cloud", routed: true, ready: true },
    ];
  });
  await page.keyboard.press("Meta+k");
  const pal = page.getByTestId("palette");
  const glm = pal.getByRole("button", { name: /Swap to: GLM Flash/ });
  await expect(glm.locator(".hint")).toHaveText("Ollama Cloud");
  await expect(glm.locator("span").first()).toHaveText("Swap to: GLM Flash");
  await expect(pal.getByRole("button", { name: /Swap to: Opus 5.5/ }).locator(".hint")).toHaveText("Anthropic");
});

test("palette shows every model row and still offers the config commands", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "one" }];
    window.__app.sessionKey = "w1:1";
    window.__app.models = Array.from({ length: 32 }, (_, i) => ({
      id: "m" + i, label: "Model " + i, provider: i < 16 ? "anthropic" : "ollama-cloud", routed: false, ready: true,
    }));
  });
  await page.keyboard.press("Meta+k");
  const pal = page.getByTestId("palette");
  await expect(pal.getByRole("button", { name: /^Swap to: / })).toHaveCount(32);
  await expect(pal.getByRole("button", { name: /^Settings\b/ })).toBeVisible();
  await expect(pal.getByRole("button", { name: /^Models\b/ })).toBeVisible();
  await expect(pal.getByRole("button", { name: /^About\b/ })).toBeVisible();
});

// The 12-row cap still binds everything that is NOT a model. Fixture has 20
// sessions (more than the cap) plus the config commands, so a cap that were
// removed would show more than 12, and one that also bit models would show
// fewer than 32 swap rows.
test("the palette cap bounds non-model rows at exactly 12 and never models", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = Array.from({ length: 20 }, (_, i) => ({ windowID: "w" + i, name: "sess" + i }));
    window.__app.sessionKey = "w0:1";
    window.__app.models = Array.from({ length: 32 }, (_, i) => ({
      id: "m" + i, label: "Model " + i, provider: i < 16 ? "anthropic" : "ollama-cloud", routed: false, ready: true,
    }));
  });
  await page.keyboard.press("Meta+k");
  const pal = page.getByTestId("palette");
  await expect(pal.getByRole("button", { name: /^Swap to: / })).toHaveCount(32);
  await expect(pal.getByRole("button", { name: /^Go: sess/ })).toHaveCount(12);
  await expect(pal.getByRole("button")).toHaveCount(44);
});

test("double-clicking the bare titlebar calls Go; double-clicking a nav button does not", async ({ page }) => {
  await page.addInitScript(() => {
    window.go = window.go || {}; window.go.main = window.go.main || {};
    window.__tbDbl = 0;
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      TitleBarDoubleClick: async () => { window.__tbDbl++; },
    });
  });
  await page.goto("/?nointro");
  const bar = await page.getByTestId("titlebar").boundingBox();
  const word = await page.getByTestId("wordmark").boundingBox();
  // An empty spot between the wordmark and the palette hint.
  await page.mouse.dblclick(word.x + word.width + 8, bar.y + bar.height / 2);
  await expect.poll(() => page.evaluate(() => window.__tbDbl)).toBe(1);
  await page.getByTestId("open-about").dblclick();
  await page.waitForTimeout(150);
  expect(await page.evaluate(() => window.__tbDbl)).toBe(1);
});
