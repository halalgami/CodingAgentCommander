import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

async function machines(page) {
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("switch-machines").click();
  await expect(page.getByTestId("machines-dialog")).toBeVisible();
}

test("machines list shows each machine's network; add a machine needs an explicit network", async ({ page }) => {
  await openRooms(page, { machines: [{ device: "mb", name: "MacBook Pro", mine: true, net: "tailscale", state: "direct (Tailscale)" }] });
  await machines(page);
  await expect(page.getByTestId("machine-row")).toContainText("MacBook Pro");
  await expect(page.getByTestId("machine-row")).toContainText("Tailscale");
  await page.getByTestId("machines-add").click();
  const cards = page.getByTestId("net-card");
  await expect(cards).toHaveCount(2);
  for (const c of await cards.all()) await expect(c).toHaveAttribute("aria-checked", "false");
  expect(await calls(page, "PairCode")).toHaveLength(0);
  await cards.filter({ hasText: "Over the internet (Tailscale)" }).click();
  expect(await calls(page, "PairCode")).toEqual([["PairCode", "tailscale"]]);
  await expect(page.getByTestId("machines-code")).toHaveValue("CMDR-ABC");
  await expect(page.getByTestId("machines-copy")).toBeVisible();
});

test("tailscale card is disabled, with the reason, when Tailscale is off", async ({ page }) => {
  await openRooms(page, { status: { setUp: true, first: true, name: "H", machine: "mini", device: "me", tailscaleUp: false } });
  await machines(page);
  await page.getByTestId("machines-add").click();
  const ts = page.getByTestId("net-card").filter({ hasText: "Tailscale" });
  await expect(ts).toBeDisabled();
  await expect(page.getByTestId("machines-dialog")).toContainText("Tailscale isn't running on this machine.");
});

test("join with a code states the network and can be cancelled", async ({ page }) => {
  await openRooms(page, { status: { setUp: false, joining: true, name: "", machine: "laptop", tailscaleUp: true } });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("switch-join").click();
  await page.getByTestId("machines-code-in").fill("CMDR-XYZ");
  await page.getByTestId("machines-join").click();
  await expect(page.getByTestId("machines-join-net")).toContainText("This code connects over Tailscale");
  await expect(page.getByTestId("machines-join-words")).toHaveText("amber · comet · lotus · tiger");
  await page.getByTestId("machines-join-cancel").click();
  expect(await calls(page, "CancelJoin")).toHaveLength(1);
});

test("invite a colleague from the room menu: explicit network", async ({ page }) => {
  await openRooms(page, { projects: SHARED, list: [{ id: "r1", name: "proj", owner: true }], view: { id: "r1", name: "proj", owner: true, threads: [], peers: [], outbox: [] } });
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await page.getByTestId("room-menu").click();
  await page.getByTestId("room-menu-invite").click();
  await page.getByTestId("net-card").filter({ hasText: "Same network (Local)" }).click();
  expect(await calls(page, "InviteCode")).toEqual([["InviteCode", "r1", "lan"]]);
  await expect(page.getByTestId("machines-code")).toHaveValue("CMDR-INV");
});

const PENDING = { kind: "pair", name: "MacBook Pro", words: ["anchor", "velvet", "orbit", "pine"], net: "tailscale", until: Math.floor(Date.now() / 1000) + 150 };

test("the 4-word check appears over anything, and Enter never allows", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("open-settings").click();
  await page.evaluate((p) => { window.__rooms.status = { ...window.__rooms.status, pending: p }; window.__roomsChanged(); }, PENDING);
  const dlg = page.getByTestId("rooms-confirm");
  await expect(dlg).toBeVisible();
  await expect(dlg).toContainText("MacBook Pro wants to join as your machine — over Tailscale");
  await expect(dlg.getByTestId("rooms-confirm-words")).toHaveText("anchor · velvet · orbit · pine");
  await page.keyboard.press("Enter");
  await page.keyboard.press(" ");
  expect(await calls(page, "ConfirmCode")).toHaveLength(0);
  await dlg.getByTestId("rooms-confirm-yes").click();
  expect(await calls(page, "ConfirmCode")).toEqual([["ConfirmCode", true]]);
});

test("Escape declines the 4-word check", async ({ page }) => {
  await openRooms(page, { status: { setUp: true, first: true, name: "H", machine: "mini", device: "me", tailscaleUp: true, pending: PENDING } });
  await expect(page.getByTestId("rooms-confirm")).toBeVisible();
  await page.keyboard.press("Escape");
  expect(await calls(page, "ConfirmCode")).toEqual([["ConfirmCode", false]]);
});

test("reset needs the word typed", async ({ page }) => {
  await openRooms(page);
  await machines(page);
  await page.getByTestId("machines-reset").click();
  const go = page.getByTestId("machines-reset-go");
  await expect(go).toBeDisabled();
  await page.getByTestId("machines-reset-word").fill("reset");
  await go.click();
  expect(await calls(page, "ResetRooms")).toHaveLength(1);
});

test("a room arriving without a folder is announced", async ({ page }) => {
  await openRooms(page);
  await page.evaluate(() => { window.__rooms.list = [{ id: "rn", name: "Ayina", needsFolder: true }]; window.__roomsChanged(); });
  await expect(page.getByText("Ayina room is now on this machine")).toBeVisible();
});

test("settings opens machines & network", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("open-settings").click();
  await page.getByTestId("settings-machines").click();
  await expect(page.getByTestId("machines-dialog")).toBeVisible();
});
