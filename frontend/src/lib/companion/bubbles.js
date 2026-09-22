// Pure bubble queue: turns finishSeq edges (from the Go overlay push) into a
// FIFO of rendered copy lines, one shown at a time for BUBBLE_MS. No three.js,
// no DOM, no Math.random — deterministic so it unit-tests under node:test and
// so a burst of finishes reads varied but reproducible.

export const BUBBLE_MS = 3500;

// Rotated by finishSeq so consecutive finishes don't repeat the same line.
const LINES = [
  (n) => `${n} is done! ✨`,
  (n) => `all wrapped up in ${n} 💫`,
  (n) => `${n}'s ready for you~`,
];

export function copyForFinish(name, seq) {
  const trimmed = name != null && String(name).trim() !== "" ? String(name) : "a session";
  const i = ((seq % LINES.length) + LINES.length) % LINES.length;
  return LINES[i](trimmed);
}

export class BubbleQueue {
  // ttlMs is injectable so a host can make the dwell user-tunable; the default
  // keeps the desktop overlay's long-standing behaviour unchanged.
  constructor(ttlMs = BUBBLE_MS) {
    this.ttlMs = ttlMs;
    this.q = [];            // rendered lines waiting to show (FIFO)
    this.shownAt = -Infinity; // nowMs the head began showing (-Infinity = none)
    this.lastSeq = 0;
    this.primed = false;
  }

  // Enqueue a line whenever finishSeq advances. The first ingest only records
  // the baseline (finishes that happened before the overlay mounted must not
  // bubble); subsequent increments enqueue.
  ingest(state) {
    const seq = state && typeof state.finishSeq === "number" ? state.finishSeq : 0;
    if (!this.primed) { this.primed = true; this.lastSeq = seq; return; }
    if (seq > this.lastSeq) {
      this.q.push(copyForFinish(state.lastFinished, seq));
      this.lastSeq = seq;
    }
  }

  // The line to display at nowMs, or null. Expires the head after ttlMs,
  // advancing to the next queued line.
  tick(nowMs) {
    if (!this.q.length) { this.shownAt = -Infinity; return null; }
    if (this.shownAt === -Infinity) {
      this.shownAt = nowMs; // start showing the head
    } else if (nowMs - this.shownAt >= this.ttlMs) {
      this.q.shift();       // head served its time
      if (!this.q.length) { this.shownAt = -Infinity; return null; }
      this.shownAt = nowMs; // start the next
    }
    return this.q[0] ?? null;
  }

  // Drop the head immediately (click-to-dismiss); the next tick shows the next.
  dismiss(nowMs) {
    if (!this.q.length) return;
    this.q.shift();
    this.shownAt = this.q.length ? nowMs : -Infinity;
  }
}
