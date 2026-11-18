import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

test("share from the card in two clicks opens the panel", async ({ page }) => {
  await openRooms(page);
  const card = page.getByTestId("session-card").first();
  await card.hover();
  await card.getByTestId("room-glyph").click();
  await expect(page.getByTestId("share-popover")).toContainText("Share “proj”");
  await expect(page.getByTestId("share-popover")).toContainText("Your other Commander machines get it when they're online; colleagues only when you invite them.");
  await page.getByTestId("share-go").click();
  expect(await calls(page, "ShareProject")).toEqual([["ShareProject", "/Users/me/proj"]]);
  await expect(page.getByTestId("room-panel")).toBeVisible();
});

test("first-time share asks for setup inline, then shares", async ({ page }) => {
  await openRooms(page, { status: { setUp: false, joining: false, name: "Hussain Algami", machine: "mini", tailscaleUp: true } });
  const card = page.getByTestId("session-card").first();
  await card.hover();
  await card.getByTestId("room-glyph").click();
  await expect(page.getByTestId("setup-name")).toHaveValue("Hussain Algami");
  await expect(page.getByTestId("setup-step")).toContainText("mini");
  expect(await calls(page, "RoomsSetup")).toHaveLength(0);
  await page.getByTestId("setup-first").click();
  expect(await calls(page, "RoomsSetup")).toEqual([["RoomsSetup", true, "Hussain Algami"]]);
  await page.getByTestId("share-go").click();
  expect(await calls(page, "ShareProject")).toHaveLength(1);
});

test("shared project: glyph shows unread without hover and opens the room", async ({ page }) => {
  await openRooms(page, { projects: SHARED, list: [{ id: "r1", name: "proj", unread: 2, online: 1, queued: 0 }],
    view: { id: "r1", name: "proj", threads: [], peers: [], outbox: [] } });
  const g = page.getByTestId("session-card").first().getByTestId("room-glyph");
  await expect(g).toBeVisible();
  await expect(g).toHaveAttribute("data-kind", "shared");
  await expect(g.getByTestId("room-glyph-unread")).toHaveText("2");
  await expect(g).toHaveAttribute("aria-label", "Room: 2 unread, 1 machine online");
  await g.click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
  expect(await calls(page, "ShareProject")).toHaveLength(0);
});

test("show in finder calls RevealFolder with the window id", async ({ page }) => {
  await openRooms(page);
  const card = page.getByTestId("session-card").first();
  await card.hover();
  await card.getByTestId("reveal-@1").click();
  expect(await calls(page, "RevealFolder")).toEqual([["RevealFolder", "@1"]]);
});

test("card right-click menu offers share and reveal", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("session-card").first().click({ button: "right" });
  const menu = page.getByTestId("card-menu");
  await menu.getByTestId("ctx-reveal").click();
  expect(await calls(page, "RevealFolder")).toEqual([["RevealFolder", "@1"]]);
  await page.getByTestId("session-card").first().click({ button: "right" });
  await page.getByTestId("card-menu").getByTestId("ctx-share").click();
  await expect(page.getByTestId("share-popover")).toBeVisible();
});
