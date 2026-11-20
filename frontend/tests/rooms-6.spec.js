import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

// Rooms phase 6: attachments, mentions, user names, outbox, budget grant.
const SHA = "ab".repeat(32);
const Q = { ref: "mb:1", author: "MacBook Pro", user: "Hasan", mine: false, own: false, kind: "question", text: "look at this", time: "2026-11-18T10:00:00Z", seen: true,
  mentionsMe: true, mentions: ["mini"], attach: [{ name: "trace.log", size: 2048, sha256: SHA, have: false }] };
const VIEW = {
  id: "r1", name: "proj", owner: true, folder: "/Users/me/proj",
  threads: [{ root: Q, replies: [], outOfBudget: true }],
  peers: [{ device: "mb", name: "MacBook Pro", net: "tailscale", state: "online" }],
  outbox: [{ ref: "me:3", text: "queued note", device: "mb", name: "MacBook Pro", state: "queued" }],
  inbox: [], drafts: [], live: [], settings: { summaries: true, budget: 5, rate: 30 },
};
const LIST = [{ id: "r1", name: "proj", owner: true, unread: 0, online: 1, queued: 0, folder: "/Users/me/proj" }];

async function openPanel(page, state = {}) {
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW, ...state });
  await expect(page.getByTestId("session-card").first()).toBeVisible();
  await page.evaluate(() => { window.__app.sessionKey = "@1:1"; window.__app.view = "terminal"; });
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
}

test("a message shows the user's name, a 'for you' mark and its attachment", async ({ page }) => {
  await openPanel(page);
  const msg = page.getByTestId("room-msg").first();
  await expect(msg).toContainText("Hasan · MacBook Pro");
  await expect(msg.getByTestId("room-mention-me")).toBeVisible();
  await msg.getByTestId("room-attachment").click();
  expect(await calls(page, "SaveRoomAttachment")).toEqual([["SaveRoomAttachment", "r1", SHA]]);
});

test("attach files and notify a machine, then send", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-kind-question").click();
  await page.getByTestId("room-mention").filter({ hasText: "MacBook Pro" }).click();
  await page.getByTestId("room-attach").click();
  await expect(page.getByTestId("room-files")).toContainText("shot.png");
  await page.getByTestId("room-send").click();
  const c = await calls(page, "SendRoomPost");
  expect(c).toHaveLength(1);
  expect(c[0][2]).toMatchObject({ kind: "question", text: "", files: ["/Users/me/shot.png"], mentions: ["mb"] });
  await expect(page.getByTestId("room-files")).toHaveCount(0);
});

test("a file alone can be sent; removing it disables Send again", async ({ page }) => {
  await openPanel(page);
  const send = page.getByTestId("room-send");
  await expect(send).toBeDisabled();
  await page.getByTestId("room-attach").click();
  await expect(send).toBeEnabled();
  await page.getByLabel("Remove shot.png").click();
  await expect(send).toBeDisabled();
});

test("outbox lists what has not reached a machine yet", async ({ page }) => {
  await openPanel(page);
  const ob = page.getByTestId("room-outbox");
  await expect(ob).toContainText("Outbox · 1 waiting");
  await ob.getByRole("button").click();
  await expect(ob).toContainText("queued: queued note");
});

test("a thread out of automatic deliveries offers 5 more", async ({ page }) => {
  await openPanel(page);
  await expect(page.getByTestId("room-budget")).toBeVisible();
  await page.getByTestId("room-grant").click();
  expect(await calls(page, "GrantThread")).toEqual([["GrantThread", "r1", "mb:1"]]);
});

test("a sidebar group whose sessions share a room shows the room badge", async ({ page }) => {
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW });
  await page.addInitScript(() => {
    const L = { groups: [{ id: "g_aaaaaaaa", name: "Work" }], order: { ungrouped: [], g_aaaaaaaa: ["@1"] } };
    Object.assign(window.go.main.App, { SidebarLayout: async () => L, SetSidebarLayout: async () => null });
  });
  await page.reload();
  const badge = page.getByTestId("group-header").getByTestId("group-room");
  await expect(badge).toHaveAttribute("title", "Room proj");
  await badge.click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
});
