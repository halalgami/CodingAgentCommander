import { test, expect } from "@playwright/test";
import { openRooms, calls, SHARED } from "./rooms-helpers.js";

// Rooms UX 1.5b: rename, copy as prompt, handoff prefill, palette, CMDR paste, Asturlab entries.
const CODE = "CMDR-" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".repeat(2);
const VIEW = {
  id: "r1", name: "proj", owner: true, folder: "/Users/me/proj",
  threads: [{ root: { ref: "mb:1", author: "MacBook Pro", mine: false, kind: "handoff", text: "Finish it", fields: { goal: "ship" }, time: "2026-11-18T10:00:00Z", seen: true }, replies: [] }],
  peers: [], outbox: [],
};
const LIST = [{ id: "r1", name: "proj", owner: true, unread: 0, online: 0, queued: 0, folder: "/Users/me/proj" }];

async function selectFirst(page) {
  await expect(page.getByTestId("session-card").first()).toBeVisible();
  await page.evaluate(() => { window.__app.sessionKey = "@1:1"; window.__app.view = "terminal"; });
  await expect(page.locator(".xterm").first()).toBeVisible();
}

async function openPanel(page, state = {}) {
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW, ...state });
  await selectFirst(page);
  await page.getByTestId("session-card").first().getByTestId("room-glyph").click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
}

function paste(page, selector, text) {
  return page.evaluate(([sel, text]) => {
    const el = sel ? document.querySelector(sel) : document.body;
    if (sel) el.focus(); else document.activeElement?.blur?.();
    const dt = new DataTransfer();
    dt.setData("text/plain", text);
    const ev = new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true });
    (sel ? el : document.body).dispatchEvent(ev);
    return ev.defaultPrevented;
  }, [selector, text]);
}

test("owner renames the room from the panel menu", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-menu").click();
  await page.getByTestId("room-menu-rename").click();
  const input = page.getByTestId("room-rename-input");
  await input.fill("Commander");
  await input.press("Enter");
  expect((await calls(page, "RenameRoom"))[0]).toEqual(["RenameRoom", "r1", "Commander"]);
  await expect(page.getByTestId("room-panel-name")).toHaveText("Commander");
});

test("copy as prompt copies the Send-to-session wrapper", async ({ page }) => {
  await page.addInitScript(() => { window.__clip = []; Object.defineProperty(navigator, "clipboard", { value: { writeText: async (t) => window.__clip.push(t) } }); });
  await openPanel(page);
  await page.getByTestId("room-msg").first().hover();
  await page.getByTestId("room-copy-prompt").first().click();
  expect(await page.evaluate(() => window.__clip)).toEqual(["[Room proj · from MacBook Pro · handoff]\nFinish it\ngoal: ship"]);
});

test("handoff prefills branch and sha from the selected session", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-kind-handoff").click();
  await expect(page.getByLabel("Branch + sha")).toHaveValue("main @ 0123456");
});

test("palette opens a room", async ({ page }) => {
  await openRooms(page, { projects: SHARED, list: LIST, view: VIEW });
  await page.getByTestId("open-palette").click();
  await page.keyboard.type("Open room: proj");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("room-panel")).toBeVisible();
});

test("palette starts a join", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("open-palette").click();
  await page.keyboard.type("Join with a code");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("machines-code-in")).toBeVisible();
});

test("palette shares an unshared session's project", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("open-palette").click();
  await page.keyboard.type("Share project");
  await page.keyboard.press("Enter");
  await expect.poll(async () => (await calls(page, "ShareProject")).length).toBe(1);
});

test("a CMDR code pasted outside the terminal opens Join with it filled in", async ({ page }) => {
  await openRooms(page);
  expect(await paste(page, null, CODE + "\n")).toBe(true);
  await expect(page.getByTestId("machines-code-in")).toHaveValue(CODE);
  expect(await calls(page, "JoinCode")).toEqual([]); // the user still presses Join
});

test("a CMDR code pasted into a text field pastes normally", async ({ page }) => {
  await openRooms(page);
  await page.getByTestId("open-palette").click();
  expect(await paste(page, "[data-testid='palette-input']", CODE)).toBe(false);
  await expect(page.getByTestId("machines-code-in")).toHaveCount(0);
});

test("a CMDR code pasted into the terminal goes through and a toast offers Join", async ({ page }) => {
  await openRooms(page);
  await selectFirst(page);
  expect(await paste(page, ".xterm-helper-textarea", CODE)).toBe(false);
  await page.getByTestId("toast-action").click();
  await expect(page.getByTestId("machines-code-in")).toHaveValue(CODE);
});

test("switcher offers recent Al Asturlab projects to share", async ({ page }) => {
  await openRooms(page, { recent: [{ identity: "rootcommit:z", root: "/Users/me/old", label: "old", roomId: "" }] });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("switch-share-recent").click();
  await expect.poll(async () => (await calls(page, "ShareProject")).map((c) => c[1])).toEqual(["/Users/me/old"]);
});

test("new room toast opens the room", async ({ page }) => {
  await openRooms(page, { projects: SHARED });
  await page.evaluate(() => { window.__rooms.list = [{ id: "r5", name: "fresh", needsFolder: true }]; window.__roomsChanged(); });
  await page.getByTestId("toast-action").click();
  await expect(page.getByTestId("room-panel-name")).toHaveText("fresh");
});

test("Al Asturlab project page shares the project, then opens its room", async ({ page }) => {
  const { AST } = await import("./asturlab-fixtures.js");
  const { pickMode } = await import("./dashboard-helpers.js");
  await page.addInitScript((ast) => { window.__ast = ast; }, AST);
  await openRooms(page, { byKey: { k1: { identity: "rootcommit:k", root: "/Users/me/ayina", label: "Ayina", roomId: "" } } });
  await pickMode(page, "asturlab");
  await page.locator('[data-testid="ast-tree-cell"][data-key="k1"]').click();
  await page.getByTestId("ast-share").click();
  await expect.poll(async () => (await calls(page, "ShareProject")).map((c) => c[1])).toEqual(["/Users/me/ayina"]);
  await page.evaluate(() => { window.__rooms.byKey.k1.roomId = "r9"; window.__roomsChanged(); });
  await page.getByTestId("ast-open-room").click();
  await expect(page.getByTestId("room-panel")).toBeVisible();
});

test("Escape cancels a rename without saving it", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-menu").click();
  await page.getByTestId("room-menu-rename").click();
  await page.getByTestId("room-rename-input").fill("Oops");
  await page.getByTestId("room-rename-input").press("Escape");
  await expect(page.getByTestId("room-panel-name")).toHaveText("proj");
  await page.waitForTimeout(300);
  expect(await calls(page, "RenameRoom")).toEqual([]);
});

test("a cleared handoff branch stays cleared", async ({ page }) => {
  await openPanel(page);
  await page.getByTestId("room-kind-handoff").click();
  const br = page.getByLabel("Branch + sha");
  await expect(br).toHaveValue("main @ 0123456");
  await br.fill("");
  await page.evaluate(() => window.__roomsChanged());
  await page.waitForTimeout(300);
  await expect(br).toHaveValue("");
});

test("a slow answer for the previous project never takes over the Share button", async ({ page }) => {
  const { AST } = await import("./asturlab-fixtures.js");
  const { pickMode } = await import("./dashboard-helpers.js");
  await page.addInitScript((ast) => { window.__ast = ast; }, AST);
  await openRooms(page, {
    byKey: { k1: { root: "/Users/me/ayina", label: "Ayina", roomId: "" }, k2: { root: "/Users/me/commander", label: "Commander", roomId: "" } },
    delay: { k1: 1200 },
  });
  await pickMode(page, "asturlab");
  await page.locator('[data-testid="ast-tree-cell"][data-key="k1"]').click();
  await page.getByTestId("ast-back").click();
  await page.locator('[data-testid="ast-tree-cell"][data-key="k2"]').click();
  await expect(page.getByTestId("ast-share")).toHaveAttribute("title", "/Users/me/commander");
  await page.waitForTimeout(1500);
  await expect(page.getByTestId("ast-share")).toHaveAttribute("title", "/Users/me/commander");
});
