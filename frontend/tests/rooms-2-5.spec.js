import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

// Rooms phases 2–5: live strip, inbox pickup, claims, settings, held drafts.
const HANDOFF = { ref: "mb:1", author: "MacBook Pro", mine: false, own: true, kind: "handoff", text: "Finish it", fields: { goal: "ship" }, time: "2026-11-18T10:00:00Z", seen: true };
const VIEW = {
  id: "r1", name: "proj", owner: true, folder: "/Users/me/proj",
  threads: [{ root: HANDOFF, replies: [] }],
  peers: [], outbox: [], inbox: [HANDOFF], drafts: [],
  live: [{ device: "mb", session: "@3", machine: "MacBook Pro", label: "api", state: "working", title: "Fix login", branch: "main", sha: "abcdef1234", online: true }],
  settings: { summaries: true, budget: 5, rate: 30 },
};
const LIST = [{ id: "r1", name: "proj", owner: true, unread: 0, online: 1, queued: 0, folder: "/Users/me/proj" }];

async function openPanel(page, state = {}) {
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW, ...state });
  await expect(page.getByTestId("session-card").first()).toBeVisible();
  await page.evaluate(() => { window.__app.sessionKey = "@1:1"; window.__app.view = "terminal"; });
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
}

test("live strip shows each running session from status events", async ({ page }) => {
  await openPanel(page);
  await expect(page.getByTestId("room-live-row")).toHaveCount(1);
  await expect(page.getByTestId("room-live-row")).toContainText("MacBook Pro");
  await expect(page.getByTestId("room-live-row")).toContainText("api · working · Fix login · main@abcdef1");
});

test("inbox: Start session opens an editable prompt and launches with it", async ({ page }) => {
  await openPanel(page);
  await expect(page.getByTestId("room-inbox-item")).toHaveCount(1);
  await page.getByTestId("room-inbox-start").click();
  const prompt = page.getByTestId("room-start-prompt");
  await expect(prompt).toHaveValue(/\[Room proj · from MacBook Pro/);
  await prompt.fill("edited prompt");
  await page.getByTestId("room-start-launch").click();
  await expect(page.getByTestId("room-start-dialog")).toHaveCount(0);
  const c = await calls(page, "StartFromMessage");
  expect(c[0].slice(1, 3)).toEqual(["r1", "mb:1"]);
  expect(c[0][4]).toBe("edited prompt");
});

test("inbox: Escape cancels Start without launching", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-inbox-start").click();
  await page.getByTestId("room-start-prompt").press("Escape");
  await expect(page.getByTestId("room-start-dialog")).toHaveCount(0);
  expect(await calls(page, "StartFromMessage")).toHaveLength(0);
});

test("inbox: Send to a running session in the project", async ({ page }) => {
  await openPanel(page, { roomSessions: [{ windowID: "@1", name: "proj" }] });
  await page.getByTestId("room-inbox-send").selectOption("@1");
  expect(await calls(page, "SendToSession")).toHaveLength(0); // choosing alone never sends
  await page.getByTestId("room-inbox-send-go").click();
  await expect.poll(() => calls(page, "SendToSession")).toEqual([["SendToSession", "r1", "mb:1", "@1"]]);
});

test("inbox: no Send to without a session in the project", async ({ page }) => {
  await openPanel(page);
  await expect(page.getByTestId("room-inbox-item")).toBeVisible();
  await expect(page.getByTestId("room-inbox-send")).toHaveCount(0);
});

test("inbox: Deliver appears only while a session waits on the thread", async ({ page }) => {
  await openPanel(page, { waiting: { "mb:1": "proj" } });
  await page.getByTestId("room-inbox-deliver").click();
  await expect.poll(() => calls(page, "DeliverToWaiting")).toEqual([["DeliverToWaiting", "r1", "mb:1"]]);
});

test("inbox: Claim and Dismiss", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-inbox-claim").click();
  await expect.poll(() => calls(page, "ClaimThread")).toEqual([["ClaimThread", "r1", "mb:1"]]);
  await page.getByTestId("room-inbox-dismiss").click();
  await expect.poll(() => calls(page, "DismissMessage")).toEqual([["DismissMessage", "r1", "mb:1"]]);
});

test("claimed thread shows who; my claim offers Release; conflict is amber", async ({ page }) => {
  const threads = [{ root: HANDOFF, replies: [], claimedBy: ["mini", "MacBook Pro"], myClaim: true, conflict: true }];
  await openPanel(page, { view: { ...VIEW, inbox: [], threads } });
  await expect(page.getByTestId("room-claim-conflict")).toContainText("mini, MacBook Pro");
  await page.getByTestId("room-msg").first().hover();
  await page.getByTestId("room-release").click();
  await expect.poll(() => calls(page, "ReleaseThread")).toEqual([["ReleaseThread", "r1", "mb:1"]]);
});

test("settings: summaries, budget and rate save for this machine", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-menu").click();
  await page.getByTestId("room-menu-settings").click();
  await page.getByTestId("room-settings-summaries").uncheck();
  await page.getByTestId("room-settings-budget").fill("8");
  await page.getByTestId("room-settings-save").click();
  await expect(page.getByTestId("room-settings")).toHaveCount(0);
  expect((await calls(page, "SetRoomSettings"))[0]).toEqual(["SetRoomSettings", "r1", { summaries: false, budget: 8, rate: 30 }]);
});

test("held agent drafts can be posted", async ({ page }) => {
  await openPanel(page, { view: { ...VIEW, drafts: [{ id: "d1", kind: "note", text: "held one" }] } });
  await expect(page.getByTestId("room-drafts")).toContainText("1 agent post held");
  await page.getByTestId("room-draft-release").click();
  await expect.poll(() => calls(page, "ReleaseDraft")).toEqual([["ReleaseDraft", "r1", "d1"]]);
});

test("composer counts bytes past 12 KB and blocks over 16 KB", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-text").fill("é".repeat(9000)); // 18 000 bytes
  await expect(page.getByTestId("room-bytes")).toContainText("/ 16 KB");
  await expect(page.getByTestId("room-send")).toBeDisabled();
});

test("Machines: remove one of my machines after a confirm", async ({ page }) => {
  await openPanel(page, { machines: [{ device: "mb", name: "MacBook Pro", mine: true, net: "tailscale", state: "direct" }] });
  await page.getByTestId("room-menu").click();
  await page.getByRole("menuitem", { name: "Machines & network…" }).click();
  await page.getByTestId("machine-remove").click();
  await expect(page.getByTestId("machine-remove-confirm")).toContainText("Remove MacBook Pro from every room?");
  expect(await calls(page, "RevokeMachine")).toHaveLength(0);
  await page.getByTestId("machine-remove-yes").click();
  await expect.poll(() => calls(page, "RevokeMachine")).toEqual([["RevokeMachine", "mb"]]);
  await expect(page.getByTestId("machine-row")).toHaveCount(0);
});

test("quit guard: Cancel during a count never quits", async ({ page }) => {
  await openPanel(page, { countDelay: 1500, queued: 0 });
  await page.evaluate(() => (window.__handlers["rooms:quitguard"] || []).forEach((f) => f(1)));
  await page.getByTestId("rooms-quitguard-wait").click();
  await page.waitForTimeout(2600); // the loop is inside QueuedCount now
  await page.getByTestId("rooms-quitguard-cancel").click();
  await page.waitForTimeout(1500);
  expect(await calls(page, "QuitAnyway")).toHaveLength(0);
});
