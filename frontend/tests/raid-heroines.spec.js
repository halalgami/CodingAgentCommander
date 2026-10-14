// frontend/tests/raid-heroines.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 40000, complexity: 800, testRatio: 0.5 } }];

async function withPack(page, pack) {
  await page.addInitScript((p) => { window.__raidPack = p; }, pack);
  await page.route("**/media/raid/**", (r) => r.fulfill({ contentType: "image/png", body: PNG }));
}

test("a taken-over heroine shows the pack image, name and tint on the Raid card", async ({ page }) => {
  await withPack(page, { dir: "/p", name: "Mine", files: { "opus-stance": "/media/raid/opus-stance-aa", "opus-attack": "/media/raid/opus-attack-bb" }, heroines: { opus: { name: "Layla", tint: "#112233" } }, warnings: [] });
  await stubDash(page, { projects: PROJ, live: [S1] });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+4");
  const her = page.locator('[data-key="@1"]').getByTestId("raid-heroine");
  await expect(her).toHaveAttribute("title", "Layla");
  await expect(her.locator("> img")).toHaveAttribute("src", /\/media\/raid\/opus-attack-bb$/);
});

test("raid:pack updates the card live", async ({ page }) => {
  await stubDash(page, { projects: PROJ, live: [{ ...S1, state: "idle" }] });
  await page.route("**/media/raid/**", (r) => r.fulfill({ contentType: "image/png", body: PNG }));
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+4");
  const her = page.locator('[data-key="@1"]').getByTestId("raid-heroine");
  await expect(her.locator("> img")).toHaveAttribute("src", /opus-(v\d-)?stance[^/]*\.webp$/);
  await page.evaluate(() => (window.__handlers["raid:pack"] || []).forEach((cb) => cb({ dir: "/p", name: "Mine", files: { "opus-stance": "/media/raid/opus-stance-cc" }, heroines: {}, warnings: [] })));
  await expect(her.locator("> img")).toHaveAttribute("src", /\/media\/raid\/opus-stance-cc$/);
});

async function openSettings(page, pack) {
  await page.addInitScript((p) => {
    window.__calls = [];
    window.__pack = p;
    const view = () => window.__pack;
    window.go = { main: { App: {
      RaidPackInfo: async () => view(),
      ListRaidPacks: async () => window.__packs || [],
      SelectRaidPack: async (d) => { window.__calls.push(["SelectRaidPack", d]); window.__pack = { ...view(), dir: d, name: "Alpha" }; return window.__pack; },
      DeleteRaidPack: async (d) => { window.__calls.push(["DeleteRaidPack", d]); window.__packs = window.__packs.filter((p) => p.dir !== d); },
      ReplaceRaidImage: async (stem) => { window.__calls.push(["ReplaceRaidImage", stem]); window.__pack = { ...view(), dir: "/p", name: "My heroines", files: { ...view().files, [stem]: "/media/raid/" + stem + "-new" } }; return window.__pack; },
      ResetRaidImage: async (stem) => { window.__calls.push(["ResetRaidImage", stem]); const f = { ...view().files }; delete f[stem]; window.__pack = { ...view(), files: f }; return window.__pack; },
      SetRaidHeroine: async (id, name, tint) => { window.__calls.push(["SetRaidHeroine", id, name, tint]); window.__pack = { ...view(), dir: "/p", heroines: { ...view().heroines, [id]: { name, tint } } }; return window.__pack; },
      ReplaceRaidImageFrom: async (stem, path) => { window.__calls.push(["ReplaceRaidImageFrom", stem, path]); return view(); },
    } } }; // only raid bindings: every other call rejects, exactly as with no Go at all
  }, pack);
  await page.route("**/media/raid/**", (r) => r.fulfill({ contentType: "image/png", body: PNG }));
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await expect(page.getByTestId("raid-heroines")).toBeVisible();
}
const EMPTY = { dir: "", name: "", files: {}, heroines: {}, warnings: [] };

test("settings grid: 30 built-in cells; replace marks a cell custom; reset reverts", async ({ page }) => {
  await openSettings(page, EMPTY);
  await expect(page.getByTestId("raid-cell")).toHaveCount(30);
  await expect(page.getByTestId("raid-pack-name")).toHaveText("Built-in heroines");
  const cell = page.locator('[data-testid="raid-cell"][data-stem="haiku-hurt"]');
  await expect(cell).toHaveAttribute("data-custom", "false");
  await cell.locator("button.pick").click();
  await expect(cell).toHaveAttribute("data-custom", "true");
  await expect(cell.locator("img")).toHaveAttribute("src", /\/media\/raid\/haiku-hurt-new$/);
  await expect(page.getByTestId("raid-pack-name")).toHaveText("My heroines");
  await cell.getByTestId("raid-cell-reset").click();
  await expect(cell).toHaveAttribute("data-custom", "false");
  expect(await page.evaluate(() => window.__calls)).toEqual([["ReplaceRaidImage", "haiku-hurt"], ["ResetRaidImage", "haiku-hurt"]]);
});

test("more images opens looks, legend and familiar cells for one heroine", async ({ page }) => {
  await openSettings(page, EMPTY);
  await page.locator('[data-testid="raid-row"][data-id="glm"]').getByTestId("raid-more").click();
  await expect(page.getByTestId("raid-cell")).toHaveCount(47);
  await expect(page.locator('[data-stem="captain-glm"]')).toHaveCount(1);
  await expect(page.locator('[data-stem="gear-familiar-glm"]')).toHaveCount(1);
});

test("name override is saved; warnings render", async ({ page }) => {
  await openSettings(page, { ...EMPTY, dir: "/p", name: "Mine", warnings: ["notes.txt: not a heroine image name; ignored"] });
  await expect(page.getByTestId("raid-pack-warnings").locator("li")).toHaveCount(1);
  const name = page.locator('[data-testid="raid-row"][data-id="opus"]').getByTestId("raid-heroine-name");
  await name.fill("Layla");
  await name.press("Enter");
  await name.blur();
  await expect.poll(() => page.evaluate(() => window.__calls.find((c) => c[0] === "SetRaidHeroine"))).toEqual(["SetRaidHeroine", "opus", "Layla", ""]);
});

test("a file dropped on a cell replaces that image", async ({ page }) => {
  await page.addInitScript(() => {
    window.runtime = Object.assign(window.runtime || {}, { OnFileDrop: (cb) => { window.__drop = cb; }, EventsOnMultiple: () => () => {}, EventsOff: () => {} });
  });
  await openSettings(page, EMPTY);
  await page.locator('[data-stem="qwen-victory"]').scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => typeof window.__drop)).toBe("function");
  const box = await page.locator('[data-stem="qwen-victory"]').boundingBox();
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/her.png"]), [box.x + 10, box.y + 10]);
  await expect.poll(() => page.evaluate(() => window.__calls.find((c) => c[0] === "ReplaceRaidImageFrom"))).toEqual(["ReplaceRaidImageFrom", "qwen-victory", "/Users/me/her.png"]);
});

// Minors round 2: dropping several files on one cell silently used the first.
test("several files dropped on one cell: the first is used and the user is told", async ({ page }) => {
  await page.addInitScript(() => {
    window.runtime = Object.assign(window.runtime || {}, { OnFileDrop: (cb) => { window.__drop = cb; }, EventsOnMultiple: () => () => {}, EventsOff: () => {} });
  });
  await openSettings(page, EMPTY);
  await page.locator('[data-stem="qwen-victory"]').scrollIntoViewIfNeeded();
  const box = await page.locator('[data-stem="qwen-victory"]').boundingBox();
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/a.png", "/Users/me/b.png", "/Users/me/c.png"]), [box.x + 10, box.y + 10]);
  await expect.poll(() => page.evaluate(() => window.__calls.filter((c) => c[0] === "ReplaceRaidImageFrom").length)).toBe(1);
  await expect(page.getByTestId("toast").last()).toContainText("a.png");
  await expect(page.getByTestId("toast").last()).toContainText(/one image/i);
});

test("pack picker: back to a pack after Use built-in; delete a non-active pack", async ({ page }) => {
  await page.addInitScript(() => {
    window.__packs = [{ dir: "/r/a", name: "Alpha", count: 3, active: false }, { dir: "/r/b", name: "Beta", count: 1, active: false }];
  });
  await openSettings(page, EMPTY);
  const sel = page.getByTestId("raid-pack-select");
  await expect(sel).toHaveValue("");
  await sel.selectOption("/r/a");
  await expect(page.getByTestId("raid-pack-name")).toHaveText("Alpha");
  const del = page.locator('[data-testid="raid-pack-delete"][data-dir="/r/b"]');
  await del.click();
  await expect(del).toHaveText("Delete for good?");
  expect(await page.evaluate(() => window.__calls.some((c) => c[0] === "DeleteRaidPack"))).toBe(false);
  await del.click();
  await expect.poll(() => page.evaluate(() => window.__calls.filter((c) => c[0] !== "ListRaidPacks"))).toEqual([["SelectRaidPack", "/r/a"], ["DeleteRaidPack", "/r/b"]]);
  await expect(page.locator('[data-testid="raid-pack-delete"]')).toHaveCount(0);
});

test("with a single pack, Use built-in can be undone from the picker", async ({ page }) => {
  await page.addInitScript(() => { window.__packs = [{ dir: "/r/a", name: "Alpha", count: 3, active: false }]; });
  await openSettings(page, EMPTY);
  await page.getByTestId("raid-pack-select").selectOption("/r/a");
  await expect(page.getByTestId("raid-pack-name")).toHaveText("Alpha");
});
