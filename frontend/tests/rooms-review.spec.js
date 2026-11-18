import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

const PENDING = { kind: "pair", name: "MacBook Pro", words: ["anchor", "velvet", "orbit", "pine"], net: "tailscale", until: Math.floor(Date.now() / 1000) + 150 };

test("review I1: Tab then Enter typed as the dialog opens never answers it", async ({ page }) => {
  await openRooms(page);
  await page.evaluate((p) => { window.__rooms.status = { ...window.__rooms.status, pending: p }; window.__roomsChanged(); }, PENDING);
  await expect(page.getByTestId("rooms-confirm")).toBeVisible();
  await page.keyboard.press("Tab");
  await page.keyboard.press("Enter");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Enter");
  expect(await calls(page, "ConfirmCode")).toHaveLength(0);
  await page.getByTestId("rooms-confirm-yes").click(); // enabled after a moment
  expect(await calls(page, "ConfirmCode")).toEqual([["ConfirmCode", true]]);
});

const stat = (cwd) => ({ cwd, model: "m", provider: "anthropic", status: "active", contextTokens: 1, contextWindow: 200000, band: "green", turns: 1 });

test("review I2: switching rooms never sends a seen receipt to the wrong room", async ({ page }) => {
  await page.addInitScript(() => { window.__focus = false; document.hasFocus = () => window.__focus; });
  await openRooms(page, {
    projects: { ...SHARED, "/Users/me/other": { identity: "rootcommit:o", root: "/Users/me/other", label: "other", roomId: "r2" } },
    list: [{ id: "r1", name: "proj", unread: 1 }, { id: "r2", name: "other" }],
    views: {
      r1: { id: "r1", name: "proj", threads: [{ root: { ref: "mb:2", author: "MacBook Pro", mine: false, kind: "note", text: "hi", time: "2026-11-18T10:00:00Z", seen: false }, replies: [] }], peers: [], outbox: [] },
      r2: { id: "r2", name: "other", threads: [], peers: [], outbox: [] },
    },
  }, { windows: [{ windowID: "@1", name: "proj" }, { windowID: "@2", name: "other" }], stats: { "@1": stat("/Users/me/proj"), "@2": stat("/Users/me/other") } });
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect(page.getByTestId("room-msg")).toHaveCount(1);
  await page.getByTestId("session-card").nth(1).getByTestId("session-name").click();
  await expect(page.getByTestId("room-panel-name")).toHaveText("other");
  await page.evaluate(() => { window.__focus = true; });
  await page.waitForTimeout(1800);
  expect(await calls(page, "MarkRoomSeen")).toHaveLength(0);
});

test("review I3: a message taller than the feed can still be seen", async ({ page }) => {
  await page.setViewportSize({ width: 1400, height: 420 });
  await page.addInitScript(() => { document.hasFocus = () => true; });
  const long = Array.from({ length: 200 }, (_, i) => "line " + i).join("\n");
  await openRooms(page, { projects: SHARED, list: [{ id: "r1", name: "proj", unread: 1 }],
    view: { id: "r1", name: "proj", threads: [{ root: { ref: "mb:9", author: "MacBook Pro", mine: false, kind: "handoff", text: long, time: "2026-11-18T10:00:00Z", seen: false }, replies: [] }], peers: [], outbox: [] } });
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect.poll(() => calls(page, "MarkRoomSeen"), { timeout: 5000 }).toEqual([["MarkRoomSeen", "r1", "mb:9"]]);
});

test("review I4: after a finished join, another code can be entered", async ({ page }) => {
  await openRooms(page, { status: { setUp: true, first: false, name: "H", machine: "laptop", tailscaleUp: true,
    join: { kind: "invite", net: "lan", words: ["a", "b", "c", "d"], status: "done" } } });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("switch-join").click();
  await expect(page.getByTestId("machines-code-in")).toBeVisible();
});

test("review I6: a room being restored stays listed, without a second Restore", async ({ page }) => {
  await openRooms(page, { hidden: [{ id: "h1", name: "Ayina", restoring: true }] });
  await page.getByTestId("open-rooms").click();
  await expect(page.getByTestId("room-switcher")).toContainText("Ayina");
  await expect(page.getByTestId("room-switcher")).toContainText("waiting for it to sync back");
  await expect(page.getByTestId("switch-restore-h1")).toHaveCount(0);
});
