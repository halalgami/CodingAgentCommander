// Al Asturlab fixture: what the Go stats engine would answer.
export const AST = {
  pricedAt: "2026-10-06",
  overview: { locked: false, overview: {
    cur: { tokens: 12_400_000, activeMs: 9_000_000, commits: 14, costUSD: 418.2, unpricedTokens: 0, planUSD: 46, planX: 9.1, cacheShare: 0.82, cacheSavedUSD: 120, pricedAt: "2026-10-06", stale: false },
    prev: { tokens: 10_000_000, activeMs: 7_200_000, commits: 10, costUSD: 300, unpricedTokens: 0, planUSD: 46, planX: 6.5, cacheShare: 0.8, cacheSavedUSD: 90, pricedAt: "2026-10-06", stale: false } } },
  rows: {
    "day:model": [{ key: "2026-11-10", split: "claude-opus-5-5", value: 100.5 }, { key: "2026-11-11", split: "claude-opus-5-5", value: 200.25 }, { key: "2026-11-11", split: "glm-5.3", value: 50 }],
    "day:provider": [{ key: "2026-11-10", split: "anthropic", value: 100.5 }],
    "day": [{ key: "2026-11-10", value: 3 }, { key: "2026-11-11", value: 5 }],
    project: [{ key: "k1", label: "Ayina", value: 9_000_000 }, { key: "k2", label: "Commander", value: 3_400_000 }],
    "weekday-hour": [{ key: "3-9", value: 4 }, { key: "0-14", value: 1 }],
    tool: [{ key: "Edit", value: 40 }, { key: "Bash", value: 22 }],
  },
  models: [
    { model: "claude-opus-5-5", provider: "anthropic", tokens: 11_000_000, turns: 80, errors: 2, compactions: 1, testRuns: 10, testFails: 1, commits: 12, lines: 900, costUSD: 418.2, priced: true },
    { model: "glm-5.3", provider: "ollama-cloud", tokens: 1_400_000, turns: 20, errors: 3, compactions: 0, testRuns: 2, testFails: 1, commits: 2, lines: 100, costUSD: 0, priced: false },
  ],
};
