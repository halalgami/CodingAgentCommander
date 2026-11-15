// Plan quota: the Go sampler's latest sample and history. This is the one
// quota source for every mode — app.quotaPct (Al Modon fog, Raid pouch) is
// fed from here, not from separate PlanUsage polls.
// The endpoint is asked ONLY when a view that shows quota opens (refreshQuota:
// ⌘8 quota card, Al Modon economy/treasury, Raid) — never on a timer (owner,
// 2026-11-15). Go skips the call if the last check is under a minute old.
import { QuotaNow, QuotaRefresh, QuotaHistory } from "../../../wailsjs/go/main/App.js";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { app } from "../stores.svelte.js";

export const quota = $state({ now: null, history: {} });

const maxUtil = (now) => {
  const xs = (now?.providers || []).filter((p) => p.supported).flatMap((p) => p.windows || []).map((w) => w.util);
  return xs.length ? Math.max(...xs) : null;
};

export async function loadQuotaNow(refresh = false) {
  try { quota.now = await (refresh ? QuotaRefresh() : QuotaNow()); } catch { return; }
  const m = maxUtil(quota.now);
  if (m != null) app.quotaPct = m;
}

export async function loadQuotaHistory(provider) {
  try { quota.history = { ...quota.history, [provider]: await QuotaHistory(provider, 168) }; } catch {}
}

let inited = false;
// refreshQuota: a quota view just opened — ask the endpoint (if stale).
export const refreshQuota = () => loadQuotaNow(true);

// initQuota runs once at startup: read what Go already knows (no network),
// then follow samples taken when a quota view opens.
export function initQuota() {
  if (inited) return;
  inited = true;
  try {
    EventsOn("quota:sample", (e) => {
      if (typeof e?.maxUtil === "number") app.quotaPct = e.maxUtil;
      QuotaNow().then((n) => (quota.now = n)).catch(() => {});
    });
  } catch {}
  loadQuotaNow();
}
