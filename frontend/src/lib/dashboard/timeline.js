// Appends dash:tick timeline marks with the backend's caps (spec §4.2): the
// last 10 minutes, at most 300 lead marks and 100 per agent per session.
const WINDOW_MS = 10 * 60 * 1000;
const MAX_LEAD = 300;
const MAX_AGENT = 100;

function capLanes(lane) {
  const lead = lane.filter((e) => !e.agent).slice(-MAX_LEAD);
  const byAgent = new Map();
  for (const e of lane) if (e.agent) (byAgent.get(e.agent) || byAgent.set(e.agent, []).get(e.agent)).push(e);
  const agents = [...byAgent.values()].flatMap((l) => l.slice(-MAX_AGENT));
  return [...lead, ...agents].sort((a, b) => a.at - b.at);
}

export function appendEvents(timeline, events, now) {
  const next = { ...timeline };
  const cutoff = now - WINDOW_MS;
  const touched = new Set();
  for (const e of events) {
    if (!touched.has(e.session)) {
      next[e.session] = (next[e.session] || []).filter((x) => x.at >= cutoff);
      touched.add(e.session);
    }
    next[e.session].push(e);
  }
  for (const id of touched) next[id] = capLanes(next[id]);
  return next;
}
