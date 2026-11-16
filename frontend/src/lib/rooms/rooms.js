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
