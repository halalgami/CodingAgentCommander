// Pure helpers for the Rooms drawer (unit-tested with node --test).

const ICONS = {
  queued: { icon: "⏳", label: "Queued — only on this machine" },
  relayed: { icon: "↗", label: "Relayed — another machine holds it" },
  delivered: { icon: "✓", label: "Delivered" },
  seen: { icon: "👁", label: "Seen" },
};
export const deliveryIcon = (s) => ICONS[s] || { icon: "?", label: s || "unknown" };

// D10: the user chooses the network explicitly; "" is shown as not chosen.
export const netLabel = (n) => (n === "lan" ? "Local network" : n === "tailscale" ? "Tailscale" : "No network chosen");

export const peerLine = (p) => `${netLabel(p.net)} · ${p.state}${p.reason ? " — " + p.reason : ""}`;

export function outboxGroups(items) {
  const by = new Map();
  for (const it of items || []) {
    if (!by.has(it.device)) by.set(it.device, { device: it.device, name: it.name, items: [] });
    by.get(it.device).items.push(it);
  }
  return [...by.values()];
}

const ORDER = ["queued", "relayed", "delivered", "seen"];
export function worst(delivery) {
  const vs = Object.values(delivery || {});
  if (!vs.length) return "";
  return vs.reduce((a, b) => (ORDER.indexOf(b) < ORDER.indexOf(a) ? b : a));
}

// ---- UX 1.5a (docs/superpowers/specs/2026-11-18-rooms-ux-design.md) ----

// A message counts as seen after this long on screen with the window focused.
export const SEEN_MS = 1000;

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

// glyphState: the session card's room icon. pr = ProjectRoom for the card's
// folder; summary = that room's RoomSummary. A closed room reads as unshared.
export function glyphState(pr, summary) {
  if (!pr?.roomId || summary?.closed) return { kind: "none", unread: 0, online: 0, queued: 0, label: "Share this project" };
  const unread = summary?.unread || 0, online = summary?.online || 0, queued = summary?.queued || 0;
  const kind = summary?.needsFolder ? "needs-folder" : "shared";
  const parts = [unread ? `${unread} unread` : "nothing new", online ? `${plural(online, "machine", "machines")} online` : "no machine online"];
  if (queued) parts.push(`${queued} not delivered`);
  if (kind === "needs-folder") parts.push("needs a folder on this machine");
  return { kind, unread, online, queued, label: "Room: " + parts.join(", ") };
}

const HAS = new Set(["delivered", "seen"]);

// deliveryLine: the line under one of my messages (spec §6.3).
export function deliveryLine(msg, peers, here = "this machine") {
  const d = msg?.mine ? Object.entries(msg.delivery || {}) : [];
  if (!d.length) return { text: "", amber: false };
  const name = (dev) => (peers || []).find((p) => p.device === dev)?.name || "another machine";
  const waiting = d.filter(([, s]) => !HAS.has(s)).map(([dev]) => name(dev));
  if (!waiting.length) return { text: "", amber: false };
  if (d.every(([, s]) => s === "queued")) return { text: `Only on ${here} — waiting for ${waiting.join(", ")}`, amber: true };
  const held = d.length - waiting.length;
  return { text: `✓ on ${held} of ${plural(d.length, "machine", "machines")} · waiting for ${waiting.join(", ")}`, amber: false };
}

// chatOrder: threads flattened oldest → newest, replies under their root.
export function chatOrder(threads) {
  const t = [...(threads || [])].sort((a, b) => String(a.root.time).localeCompare(String(b.root.time)));
  const items = [];
  for (const th of t) {
    items.push({ msg: th.root, depth: 0 });
    for (const r of [...(th.replies || [])].sort((a, b) => String(a.time).localeCompare(String(b.time)))) items.push({ msg: r, depth: 1 });
  }
  return { items, firstUnread: items.findIndex((x) => !x.msg.mine && !x.msg.seen) };
}

export function revealLabel(ua = globalThis.navigator?.userAgent || "") {
  if (/Mac/.test(ua)) return "Show in Finder";
  if (/Win/.test(ua)) return "Show in File Explorer";
  return "Open folder";
}

// panelMode: dock beside the terminal unless that leaves it under 600px.
export const panelMode = (windowW, sidebarW, panelW) => (windowW - sidebarW - panelW < 600 ? "overlay" : "dock");
