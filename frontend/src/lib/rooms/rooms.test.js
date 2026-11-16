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
