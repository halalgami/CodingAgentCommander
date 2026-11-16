import { test, expect } from "@playwright/test";
import { stubDash } from "./dashboard-helpers.js";

const VIEW = {
  id: "r1", name: "Ayina", owner: true, folder: "/p/ayina", unread: 1, queued: 0,
  threads: [
    { root: { ref: "me:3", author: "mini", mine: true, kind: "handoff", text: "run the windows tests", time: "2026-11-16T09:00:00Z", seen: true, fields: { goal: "green" }, delivery: { pc: "relayed", lap: "seen" } }, replies: [
      { ref: "pc:4", parent: "me:3", author: "wafas-pc", mine: false, kind: "result", text: "all green", time: "2026-11-16T10:00:00Z", seen: false } ], last: "x" },
  ],
  peers: [
    { device: "pc", name: "wafas-pc", mine: true, net: "lan", state: "direct (Local network)", head: 4, waiting: 1 },
    { device: "col", name: "colleague-laptop", mine: false, net: "", state: "unreachable", reason: "choose a network for this machine", head: 0, waiting: 0 },
  ],
  outbox: [{ ref: "me:3", text: "run the windows tests", device: "pc", name: "wafas-pc", state: "relayed" }],
};

async function open(page, state = {}) {
  await stubDash(page, {});
  await page.addInitScript((state) => {
    const R = window.__rooms = Object.assign({ status: { setUp: true, first: true, name: "Hussain", device: "me", tailscaleUp: false }, list: [], view: null }, state);
    const rec = (n, ...a) => window.__calls.push([n, ...a]);
    Object.assign(window.go.main.App, {
      RoomsStatus: async () => R.status,
      Rooms: async () => R.list,
      RoomView: async () => R.view,
      RoomsSetup: async (...a) => { rec("RoomsSetup", ...a); R.status = { ...R.status, setUp: a[0], joining: !a[0] }; },
      PairCode: async (n) => { rec("PairCode", n); return { code: "CMDR-ABC", net: n, expires: 1900000000 }; },
      InviteCode: async (r, n) => { rec("InviteCode", r, n); return { code: "CMDR-INV", net: n, expires: 1900000000 }; },
      JoinCode: async (...a) => { rec("JoinCode", ...a); R.status = { ...R.status, join: { kind: "pair", net: "lan", words: ["amber", "comet", "lotus", "tiger"], status: "waiting" } }; return { words: ["amber", "comet", "lotus", "tiger"] }; },
      ConfirmCode: async (ok) => { rec("ConfirmCode", ok); R.status = { ...R.status, pending: null }; },
      CancelCode: async () => rec("CancelCode"),
      CreateRoom: async (...a) => { rec("CreateRoom", ...a); return { id: "r2", name: "x" }; },
      PickRoomFolder: async () => "/picked",
      PostRoomMessage: async (...a) => rec("PostRoomMessage", ...a),
      MarkRoomSeen: async (...a) => rec("MarkRoomSeen", ...a),
      SetDeviceNetwork: async (...a) => rec("SetDeviceNetwork", ...a),
      BindRoomFolder: async (...a) => rec("BindRoomFolder", ...a),
      QueuedCount: async () => 0,
      QuitAnyway: async () => rec("QuitAnyway"),
    });
  }, state);
  await page.goto("/?nointro");
  await expect(page.getByTestId("open-rooms")).toBeVisible();
}
const calls = (page, n) => page.evaluate((n) => window.__calls.filter((c) => c[0] === n), n);

test("first run: first machine or pair; nothing set up before a choice", async ({ page }) => {
  await open(page, { status: { setUp: false, joining: false, tailscaleUp: false } });
  await page.getByTestId("open-rooms").click();
  await expect(page.getByTestId("rooms-firstrun")).toBeVisible();
  expect(await calls(page, "RoomsSetup")).toHaveLength(0);
  await page.getByTestId("rooms-name").fill("Hussain");
  await page.getByTestId("rooms-first").click();
  expect(await calls(page, "RoomsSetup")).toEqual([["RoomsSetup", true, "Hussain"]]);
});

test("pairing a second machine shows the 4 words", async ({ page }) => {
  await open(page, { status: { setUp: false, joining: true, tailscaleUp: false } });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("rooms-code-in").fill("CMDR-ABC");
  await page.getByTestId("rooms-join").click();
  await expect(page.getByTestId("rooms-join-words")).toHaveText("amber · comet · lotus · tiger");
  await expect(page.getByTestId("rooms-join-net")).toContainText("Local network");
});

test("a code needs an explicit network; Tailscale is disabled when it is off", async ({ page }) => {
  await open(page, { list: [{ id: "r1", name: "Ayina", unread: 0, queued: 0 }], view: VIEW });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("rooms-codes").locator("summary").click();
  await expect(page.getByTestId("rooms-netpick").locator('input[value="tailscale"]')).toBeDisabled();
  await page.getByTestId("rooms-pair-code").click();
  await expect(page.getByTestId("rooms-error")).toContainText("Choose the network first");
  expect(await calls(page, "PairCode")).toHaveLength(0);
  await page.getByTestId("rooms-netpick").getByLabel("Local network").check();
  await page.getByTestId("rooms-invite-code").click();
  expect(await calls(page, "InviteCode")).toEqual([["InviteCode", "r1", "lan"]]);
  await expect(page.getByTestId("rooms-code-out")).toHaveValue("CMDR-INV");
});

test("host confirms after comparing words", async ({ page }) => {
  await open(page, { status: { setUp: true, first: true, tailscaleUp: true, pending: { kind: "pair", name: "wafas-pc", words: ["a", "b", "c", "d"] } } });
  await page.getByTestId("open-rooms").click();
  await expect(page.getByTestId("rooms-confirm-words")).toHaveText("a · b · c · d");
  await page.getByTestId("rooms-confirm-yes").click();
  expect(await calls(page, "ConfirmCode")).toEqual([["ConfirmCode", true]]);
});

test("room: threads, delivery icon, inbox, outbox, people with network", async ({ page }) => {
  await open(page, { list: [{ id: "r1", name: "Ayina", unread: 1, queued: 1 }], view: VIEW });
  await expect(page.getByTestId("rooms-badge")).toHaveText("1");
  await expect(page.getByTestId("rooms-queued-dot")).toBeVisible();
  await page.getByTestId("open-rooms").click();
  await expect(page.getByTestId("rooms-msg")).toHaveCount(2);
  await expect(page.getByTestId("rooms-delivery")).toHaveText("↗"); // worst of relayed/seen
  await page.getByTestId("rooms-tab-inbox").click();
  await page.getByTestId("rooms-mark-seen").click();
  expect(await calls(page, "MarkRoomSeen")).toEqual([["MarkRoomSeen", "r1", "pc:4"]]);
  await page.getByTestId("rooms-tab-outbox").click();
  await expect(page.getByTestId("rooms-outbox-group")).toContainText("1 waiting for wafas-pc");
  await page.getByTestId("rooms-tab-people").click();
  const rows = page.getByTestId("rooms-peer");
  await expect(rows.nth(1)).toContainText("choose a network");
  await rows.nth(1).getByTestId("rooms-peer-net").selectOption("tailscale");
  expect(await calls(page, "SetDeviceNetwork")).toEqual([["SetDeviceNetwork", "col", "tailscale"]]);
});

test("composer: handoff fields and send", async ({ page }) => {
  await open(page, { list: [{ id: "r1", name: "Ayina" }], view: VIEW });
  await page.getByTestId("open-rooms").click();
  await page.getByTestId("rooms-kind").selectOption("handoff");
  await page.getByLabel("Goal").fill("fix tests");
  await page.getByTestId("rooms-text").fill("please run them");
  await page.getByTestId("rooms-send").click();
  const c = await calls(page, "PostRoomMessage");
  expect(c).toHaveLength(1);
  expect(c[0].slice(1, 5)).toEqual(["r1", "handoff", "please run them", ""]);
  expect(c[0][5].goal).toBe("fix tests");
});

test("quit guard: shown on the event, Quit anyway quits", async ({ page }) => {
  await open(page, { list: [], view: null });
  await page.evaluate(() => (window.__handlers["rooms:quitguard"] || []).forEach((f) => f(2)));
  await expect(page.getByTestId("rooms-quitguard")).toContainText("2 messages have not left this machine");
  await page.getByTestId("rooms-quitguard-quit").click();
  expect(await calls(page, "QuitAnyway")).toHaveLength(1);
});
