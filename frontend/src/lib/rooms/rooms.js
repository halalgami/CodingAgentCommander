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

// ---- UX 1.5b ----

// promptOf: the wrapper phase 3's "Send to session" pastes (spec §13).
// cleanText keeps newlines and drops every other control character (ESC,
// CR, BEL, DEL, C1): copied room text ends up pasted into a terminal.
export function cleanText(s) {
  return String(s ?? "").replace(/\r\n/g, "\n").replace(/[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u2028\u2029]/g, "");
}
const oneLine = (s) => cleanText(s).replace(/\n/g, " ");

export function promptOf(msg, roomName) {
  const lines = [`[Room ${oneLine(roomName)} · from ${oneLine(msg.author)} · ${oneLine(msg.kind)}]`, cleanText(msg.text)];
  for (const [k, v] of Object.entries(msg.fields || {})) if (v) lines.push(`${oneLine(k)}: ${oneLine(v)}`);
  return lines.join("\n");
}

// cmdrCode: the code, if the whole pasted text is one Commander code
// (line breaks from chat apps removed); otherwise "".
export function cmdrCode(text) {
  const t = String(text || "").replace(/\s+/g, "");
  return /^CMDR-[A-Z2-7]{24,}$/.test(t) ? t : "";
}

// pasteVerdict: what a CMDR paste does where focus is (spec §4, §15.5).
// "terminal" = let it through and offer Join in a toast; "join" = offer Join;
// "ignore" = a text field — paste exactly as before.
export function pasteVerdict(activeEl, text) {
  if (!cmdrCode(text)) return "ignore";
  if (activeEl?.closest?.(".xterm")) return "terminal";
  const tag = activeEl?.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || activeEl?.isContentEditable) return "ignore";
  return "join";
}

// handoffBranch: "branch @ sha7" for the handoff prefill.
export function handoffBranch(head) {
  if (!head?.sha) return "";
  const sha = head.sha.slice(0, 7);
  return head.branch && head.branch !== "HEAD" ? `${head.branch} @ ${sha}` : sha;
}

// ---- phases 2–5 ----

// liveLine: one line per running session in the live strip (rooms spec §11).
export function liveLine(l) {
  const parts = [l.label || "session", l.state || "idle"];
  if (l.title) parts.push(l.title);
  if (l.branch) parts.push(l.branch + (l.sha ? "@" + l.sha.slice(0, 7) : "") + (l.dirty ? "*" : ""));
  if (l.listening) parts.push("listening on " + l.listening.slice(-10));
  return parts.join(" · ");
}

// byteLen counts UTF-8 bytes (the 16 KB message limit is in bytes).
export function byteLen(s) {
  return new TextEncoder().encode(s || "").length;
}

// inboxActions: what an inbox item offers (spec §10.5). waiting = a session
// here is in room_wait on the item's thread.
export function inboxActions(m, { waiting = false, sessions = 0, folder = true } = {}) {
  const out = [];
  if (waiting) out.push("deliver");
  if (folder) out.push("start");
  if (sessions > 0) out.push("send");
  if (m.kind === "handoff" || m.kind === "question") out.push("claim");
  out.push("reply", "dismiss");
  return out;
}

// ---- phase 6: attachments, mentions, user names ----
export const baseName = (p) => String(p || "").split(/[\\/]/).filter(Boolean).pop() || "";
export function fmtSize(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1 << 20)).toFixed(1)} MB`;
}
export const authorLine = (m) => (m.user ? `${m.user} · ${m.author}` : m.author);
export const canSend = (text, files, bytes, busy) => !busy && bytes <= 16384 && (!!text.trim() || (files?.length || 0) > 0);

// groupRoomId (spec §11): a sidebar group shows the room badge when every
// session in it is in the same room's project.
export function groupRoomId(cwds, roomOf) {
  let id = "";
  for (const c of cwds) {
    const r = roomOf(c);
    if (!r || (id && r !== id)) return "";
    id = r;
  }
  return id;
}
