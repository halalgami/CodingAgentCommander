import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

const msg = (ref, extra = {}) => ({ ref, author: "mini", mine: true, kind: "note", text: "m " + ref, time: "2026-11-18T09:00:00Z", seen: true, ...extra });
const VIEW = {
  id: "r1", name: "proj", owner: true, folder: "/Users/me/proj",
  threads: [
    { root: msg("me:1", { delivery: { mb: "queued" } }), replies: [] },
    { root: msg("mb:2", { mine: false, author: "MacBook Pro", time: "2026-11-18T10:00:00Z", seen: false }), replies: [] },
  ],
  peers: [
    { device: "mb", name: "MacBook Pro", mine: true, net: "tailscale", state: "direct (Tailscale)", head: 2, waiting: 1 },
    { device: "pc", name: "WafasGrace", mine: true, net: "tailscale", state: "unreachable", reason: "it reached this machine over Local network, but you chose Tailscale", tried: "lan", head: 0, waiting: 0 },
  ],
  outbox: [],
};
const LIST = [{ id: "r1", name: "proj", owner: true, unread: 1, online: 1, queued: 1, folder: "/Users/me/proj" }];

async function openPanel(page, state = {}, dash = {}) {
  await page.addInitScript(() => { document.hasFocus = () => window.__focus ?? true; });
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW, ...state }, dash);
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
}

test("panel docks beside the terminal, which stays visible", async ({ page }) => {
  await openPanel(page);
  await expect(page.getByTestId("room-panel")).toHaveAttribute("data-mode", "dock");
  const pane = await page.locator("section.pane").boundingBox();
  const panel = await page.getByTestId("room-panel").boundingBox();
  expect(pane.width).toBeGreaterThanOrEqual(600);
  expect(panel.x).toBeGreaterThanOrEqual(pane.x + pane.width - 1);
});

test("panel overlays the terminal on a narrow window", async ({ page }) => {
  await page.setViewportSize({ width: 1000, height: 720 });
  await openPanel(page);
  await expect(page.getByTestId("room-panel")).toHaveAttribute("data-mode", "overlay");
});

test("panel follows the selected session's project", async ({ page }) => {
  const stats = {
    "@1": { cwd: "/Users/me/proj", model: "m", provider: "anthropic", status: "active", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1 },
    "@2": { cwd: "/Users/me/other", model: "m", provider: "anthropic", status: "active", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1 },
  };
  await openPanel(page, {
    projects: { ...SHARED, "/Users/me/other": { identity: "rootcommit:o", root: "/Users/me/other", label: "other", roomId: "r2" } },
    list: [...LIST, { id: "r2", name: "other", owner: true, unread: 0, online: 0, queued: 0, folder: "/Users/me/other" }],
    views: { r1: VIEW, r2: { id: "r2", name: "other", owner: true, threads: [], peers: [], outbox: [] } },
  }, { windows: [{ windowID: "@1", name: "proj" }, { windowID: "@2", name: "other" }], stats });
  await expect(page.getByTestId("room-panel-name")).toHaveText("proj");
  await page.getByTestId("session-card").nth(1).getByTestId("session-name").click();
  await expect(page.getByTestId("room-panel-name")).toHaveText("other");
});

test("New divider; seen only after a second visible with the window focused", async ({ page }) => {
  await page.addInitScript(() => { window.__focus = false; });
  await openPanel(page);
  await expect(page.getByTestId("room-new-divider")).toBeVisible();
  await page.waitForTimeout(1600);
  expect(await calls(page, "MarkRoomSeen")).toHaveLength(0);
  await page.evaluate(() => { window.__focus = true; });
  await expect.poll(() => calls(page, "MarkRoomSeen")).toEqual([["MarkRoomSeen", "r1", "mb:2"]]);
});

test("my undelivered message says it is only here", async ({ page }) => {
  await openPanel(page);
  const line = page.getByTestId("room-delivery").first();
  await expect(line).toContainText("Only on this");
  await expect(line).toContainText("waiting for MacBook Pro");
  await expect(line).toHaveAttribute("data-amber", "true");
});

test("refused machine: the switch is offered, never made", async ({ page }) => {
  await openPanel(page);
  const chip = page.getByTestId("room-chip").filter({ hasText: "WafasGrace" });
  await expect(chip).toContainText("Tailscale");
  expect(await calls(page, "SetDeviceNetwork")).toHaveLength(0);
  await chip.getByTestId("room-chip-switch").click();
  expect(await calls(page, "SetDeviceNetwork")).toEqual([["SetDeviceNetwork", "pc", "lan"]]);
});

test("composer: chips, Enter sends, Shift+Enter is a new line", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-kind-handoff").click();
  await page.getByLabel("Goal").fill("fix tests");
  const box = page.getByTestId("room-text");
  await box.fill("line one");
  await box.press("Shift+Enter");
  await box.type("line two");
  await box.press("Enter");
  const c = await calls(page, "SendRoomPost");
  expect(c).toHaveLength(1);
  expect(c[0][1]).toBe("r1");
  expect(c[0][2]).toMatchObject({ kind: "handoff", text: "line one\nline two", parent: "", files: [], mentions: [] });
  expect(c[0][2].fields.goal).toBe("fix tests");
});

test("close room: confirmation warns, then the room is read-only", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-menu").click();
  await page.getByTestId("room-menu-close").click();
  const dlg = page.getByTestId("room-close-dialog");
  await expect(dlg).toContainText("Closes proj for everyone");
  await expect(dlg).toContainText("1 message hasn't reached");
  await dlg.getByTestId("room-close-confirm").click();
  expect(await calls(page, "CloseRoom")).toEqual([["CloseRoom", "r1"]]);
  await expect(page.getByTestId("room-closed-note")).toContainText("Closed by mini");
  await expect(page.getByTestId("room-text")).toHaveCount(0);
});

test("a room without a folder here offers the matching folder", async ({ page }) => {
  await openPanel(page, { list: [{ ...LIST[0], needsFolder: true, folder: "" }], suggest: [{ path: "/Users/me/proj", label: "proj", source: "session" }] });
  await page.getByTestId("room-suggest").first().click();
  expect(await calls(page, "BindRoomFolder")).toEqual([["BindRoomFolder", "r1", "/Users/me/proj"]]);
});

test("switcher: share candidates once per project; join with a code", async ({ page }) => {
  const stats = {
    "@1": { cwd: "/Users/me/proj", model: "m", provider: "anthropic", status: "active", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1 },
    "@2": { cwd: "/Users/me/proj/.worktrees/x", model: "m", provider: "anthropic", status: "active", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1 },
  };
  const p = { identity: "rootcommit:abc", root: "/Users/me/proj", label: "proj", roomId: "" };
  await openRooms(page, { projects: { "/Users/me/proj": p, "/Users/me/proj/.worktrees/x": p }, hidden: [{ id: "h1", name: "old" }] },
    { windows: [{ windowID: "@1", name: "a" }, { windowID: "@2", name: "b" }], stats });
  await page.getByTestId("open-rooms").click();
  await expect(page.getByTestId("switch-share")).toHaveCount(1);
  await page.getByTestId("switch-restore-h1").click();
  expect(await calls(page, "RestoreRoom")).toEqual([["RestoreRoom", "h1"]]);
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("switch-share").click();
  expect(await calls(page, "ShareProject")).toEqual([["ShareProject", "/Users/me/proj"]]);
});

test("quit guard: shown on the event, Quit anyway quits", async ({ page }) => {
  await openRooms(page);
  await page.evaluate(() => (window.__handlers["rooms:quitguard"] || []).forEach((f) => f(2)));
  await expect(page.getByTestId("rooms-quitguard")).toContainText("2 messages have not left this machine");
  await page.getByTestId("rooms-quitguard-quit").click();
  expect(await calls(page, "QuitAnyway")).toHaveLength(1);
});
