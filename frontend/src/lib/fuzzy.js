// Tiny fuzzy subsequence matcher for the command palette. Scoring favors
// consecutive runs and word starts; 0 means no match.
export function fuzzyScore(query, text) {
  if (!query) return 1;
  const q = query.toLowerCase(), t = text.toLowerCase();
  let qi = 0, score = 0, streak = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      streak++;
      const wordStart = ti === 0 || /[\s\-_/.]/.test(t[ti - 1]);
      score += 1 + streak * 2 + (wordStart ? 4 : 0);
      qi++;
    } else {
      streak = 0;
    }
  }
  return qi === q.length ? score : 0;
}

export function fuzzyFilter(query, items, keyFn) {
  return items
    .map((it) => [fuzzyScore(query, keyFn(it)), it])
    .filter(([s]) => s > 0)
    .sort((a, b) => b[0] - a[0])
    .map(([, it]) => it);
}
