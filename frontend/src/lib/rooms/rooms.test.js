import { test } from "node:test";
import assert from "node:assert/strict";
import { deliveryIcon, netLabel, peerLine, outboxGroups, worst } from "./rooms.js";

test("deliveryIcon maps every state (spec §9)", () => {
  assert.deepEqual(deliveryIcon("queued"), { icon: "⏳", label: "Queued — only on this machine" });
  assert.equal(deliveryIcon("relayed").icon, "↗");
  assert.equal(deliveryIcon("delivered").icon, "✓");
  assert.equal(deliveryIcon("seen").icon, "👁");
  assert.equal(deliveryIcon("weird").icon, "?");
});

test("netLabel names the chosen network, never a default", () => {
  assert.equal(netLabel("lan"), "Local network");
  assert.equal(netLabel("tailscale"), "Tailscale");
  assert.equal(netLabel(""), "No network chosen");
});

test("peerLine shows network, state and reason", () => {
  assert.equal(peerLine({ net: "lan", state: "direct (Local network)" }), "Local network · direct (Local network)");
  assert.equal(peerLine({ net: "tailscale", state: "unreachable", reason: "Tailscale is off on this machine" }), "Tailscale · unreachable — Tailscale is off on this machine");
});

test("outboxGroups groups by device with counts", () => {
  const g = outboxGroups([{ device: "d1", name: "pc", ref: "a", state: "queued" }, { device: "d1", name: "pc", ref: "b", state: "relayed" }, { device: "d2", name: "mac", ref: "a", state: "queued" }]);
  assert.deepEqual(g.map((x) => [x.name, x.items.length]), [["pc", 2], ["mac", 1]]);
});

test("worst picks the least-delivered state for a message", () => {
  assert.equal(worst({ a: "seen", b: "queued" }), "queued");
  assert.equal(worst({ a: "seen", b: "delivered" }), "delivered");
  assert.equal(worst({}), "");
});

// ---- UX 1.5a ----
import { glyphState, deliveryLine, chatOrder, revealLabel, panelMode, SEEN_MS } from "./rooms.js";

test("glyphState: unshared, shared, needs a folder, closed", () => {
  assert.equal(glyphState(undefined, undefined).kind, "none");
  assert.equal(glyphState({ roomId: "" }, undefined).label, "Share this project");
  const g = glyphState({ roomId: "r1" }, { id: "r1", unread: 2, online: 1, queued: 0 });
  assert.deepEqual([g.kind, g.unread, g.online], ["shared", 2, 1]);
  assert.equal(g.label, "Room: 2 unread, 1 machine online");
  assert.equal(glyphState({ roomId: "r1" }, { id: "r1", needsFolder: true }).kind, "needs-folder");
  assert.equal(glyphState({ roomId: "r1" }, { id: "r1", closed: true }).kind, "none");
  assert.equal(glyphState({ roomId: "r1" }, { id: "r1", unread: 0, online: 0, queued: 3 }).label, "Room: nothing new, no machine online, 3 not delivered");
});

const PEERS = [{ device: "mb", name: "MacBook Pro" }, { device: "pc", name: "Ahmed's PC" }, { device: "w", name: "WafasGrace" }];

test("deliveryLine: everyone has it → nothing", () => {
  assert.deepEqual(deliveryLine({ mine: true, delivery: { mb: "seen", pc: "delivered" } }, PEERS), { text: "", amber: false });
});

test("deliveryLine: partly delivered", () => {
  assert.deepEqual(deliveryLine({ mine: true, delivery: { mb: "seen", pc: "relayed", w: "delivered" } }, PEERS),
    { text: "✓ on 2 of 3 machines · waiting for Ahmed's PC", amber: false });
});

test("deliveryLine: only here is amber", () => {
  assert.deepEqual(deliveryLine({ mine: true, delivery: { mb: "queued" } }, PEERS, "this Mac"),
    { text: "Only on this Mac — waiting for MacBook Pro", amber: true });
  assert.deepEqual(deliveryLine({ mine: false, delivery: { mb: "queued" } }, PEERS), { text: "", amber: false });
});

test("chatOrder: oldest first, replies indented, first unread", () => {
  const m = (ref, time, extra = {}) => ({ ref, time, mine: true, seen: true, ...extra });
  const threads = [
    { root: m("b", "2026-11-18T10:00:00Z"), replies: [m("b2", "2026-11-18T11:00:00Z", { mine: false, seen: false })] },
    { root: m("a", "2026-11-18T09:00:00Z"), replies: [] },
  ];
  const { items, firstUnread } = chatOrder(threads);
  assert.deepEqual(items.map((x) => [x.msg.ref, x.depth]), [["a", 0], ["b", 0], ["b2", 1]]);
  assert.equal(firstUnread, 2);
  assert.equal(chatOrder([]).firstUnread, -1);
});

test("revealLabel names the platform's file manager", () => {
  assert.equal(revealLabel("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"), "Show in Finder");
  assert.equal(revealLabel("Mozilla/5.0 (Windows NT 10.0; Win64; x64)"), "Show in File Explorer");
  assert.equal(revealLabel("Mozilla/5.0 (X11; Linux x86_64)"), "Open folder");
});

test("panelMode overlays when the terminal would drop below 600px", () => {
  assert.equal(panelMode(1250, 290, 360), "dock");
  assert.equal(panelMode(1249, 290, 360), "overlay");
  assert.equal(SEEN_MS, 1000);
});
