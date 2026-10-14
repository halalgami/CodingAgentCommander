// River geometry (spec §6 B): time → x on a three-row staff, tributaries for
// agents, gaps for permission waits. Pure, so it is node-testable.
export function xOf(at, now, windowMs, width) {
  const x = width - ((now - at) / windowMs) * width;
  return Math.max(0, Math.min(width, x));
}

const ROW = { read: 0, web: 0, other: 0, edit: 1, think: 1, agent: 1, shell: 2 };
export const rowOf = (kind) => ROW[kind] ?? 0;

const inWindow = (e, now, windowMs) => e.at >= now - windowMs;

export function layoutLane(events, now, windowMs, width) {
  return events
    .filter((e) => !e.agent && e.kind !== "wait" && inWindow(e, now, windowMs))
    .map((e) => ({ x: xOf(e.at, now, windowMs, width), row: rowOf(e.kind), kind: e.kind, label: e.label || "", agent: "" }));
}

export function tributaries(crew, events, now, windowMs, width) {
  const visible = crew.filter((a) => !a.endedMs || a.endedMs >= now - windowMs);
  const open = visible.filter((a) => !a.endedMs);
  // A tributary's curve needs 24 px before the river head: a new agent starts there.
  const start = (ms) => Math.min(xOf(ms || now, now, windowMs, width), width - 24);
  const lane = (a) => ({
    agent: a.id, background: !!a.background, braid: false, count: 1,
    x0: start(a.startedMs),
    x1: a.endedMs ? xOf(a.endedMs, now, windowMs, width) : null,
    glyphs: events.filter((e) => e.agent === a.id && inWindow(e, now, windowMs))
      .map((e) => ({ x: xOf(e.at, now, windowMs, width), kind: e.kind, label: e.label || "" })),
  });
  if (open.length <= 3) return visible.map(lane);
  const ended = visible.filter((a) => a.endedMs).map(lane);
  const x0 = Math.min(...open.map((a) => start(a.startedMs)));
  return [...ended, { agent: "", background: false, braid: true, count: open.length, x0, x1: null, glyphs: [] }];
}

export function fmtDuration(ms) {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

export function waitGaps(events, row, now, windowMs, width) {
  const gaps = [];
  let start = null;
  for (const e of events) {
    if (e.agent || e.kind !== "wait") continue;
    if (e.label === "start") start = e.at;
    else if (e.label === "end" && start != null) {
      if (e.at >= now - windowMs) gaps.push({ x0: xOf(start, now, windowMs, width), x1: xOf(e.at, now, windowMs, width), label: "" });
      start = null;
    }
  }
  if (row.activity === "permission" && row.sinceMs) {
    gaps.push({ x0: xOf(row.sinceMs, now, windowMs, width), x1: width, label: `Waiting ${fmtDuration(now - row.sinceMs)}` });
  }
  return gaps;
}

export function sparkline(events, now, windowMs, buckets) {
  const out = new Array(buckets).fill(0);
  for (const e of events) {
    if (e.kind === "think" || e.kind === "wait" || e.at < now - windowMs || e.at > now) continue;
    const i = Math.min(buckets - 1, Math.floor(((e.at - (now - windowMs)) / windowMs) * buckets));
    out[i]++;
  }
  return out;
}
