// Pure decision layer: a per-frame snapshot -> { expression, gesture }.
// Edge-detected one-shot gestures (fire on a transition, not every frame),
// priority-ordered, with a global gesture cooldown. No three.js imports.

const COOLDOWN_MS = 600;
const IDLE_BORED_MS = 5 * 60 * 1000;
const BORED_GAP_MS = 20000;

// Per-region reactions (side-aware). Values are pools of gesture names (must
// exist in gestures.js GESTURES); one member is picked per fire via the
// injected rng for variety. Singleton pools always resolve to that one name.
const CLICK_GESTURE = {
  head: ["headpat"],
  handL: ["waveL"], handR: ["waveR"],
  armL: ["nudgeL"], armR: ["nudgeR"],
  legL: ["kickL"], legR: ["kickR"],
  chest: ["bashful"], belly: ["tickle", "squirm"], hips: ["wiggle"],
};
const CLICK_EXPR = {
  head: "happy",
  handL: "happy", handR: "happy",
  armL: "relaxed", armR: "relaxed",
  legL: "surprised", legR: "surprised",
  chest: "surprised", belly: "happy", hips: "surprised",
};

export class Reactions {
  constructor(rng = Math.random) { this.rng = rng; this.prev = null; this.lastGestureMs = -Infinity; this.lastBoredMs = -Infinity; }

  decide(s) {
    const prev = this.prev;
    // baseline mood
    let expression = "neutral";
    if (s.quotaHigh) expression = "sad";
    else if (s.running > 0) expression = "relaxed";

    let gesture = null;
    const rose = (k) => (prev ? s[k] > prev[k] : s[k] > 0);
    const edge = (k) => s[k] && !(prev && prev[k]);

    if (edge("error")) { gesture = "slump"; expression = "surprised"; }
    else if (s.click) {
      const pool = CLICK_GESTURE[s.click];
      gesture = pool ? pool[Math.floor(this.rng() * pool.length) % pool.length] : "startle_hop";
      expression = CLICK_EXPR[s.click] ?? "surprised";
    }
    else if (edge("rc")) { gesture = "bye"; expression = "happy"; }
    else if (rose("finished") || edge("awaiting")) { gesture = "wave"; expression = "happy"; }
    else if (!s.greeted) { gesture = "greet"; expression = "happy"; }
    else if (prev && prev.running === 0 && s.running > 0) { expression = "relaxed"; }
    else if (s.running === 0 && s.idleMs >= IDLE_BORED_MS && s.nowMs - this.lastBoredMs > BORED_GAP_MS) {
      gesture = "stretch"; this.lastBoredMs = s.nowMs;
    }

    if (gesture && s.nowMs - this.lastGestureMs < COOLDOWN_MS) gesture = null;
    if (gesture) this.lastGestureMs = s.nowMs;

    this.prev = s;
    return { expression, gesture };
  }
}
