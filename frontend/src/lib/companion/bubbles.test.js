import { test } from "node:test";
import assert from "node:assert/strict";
import { BubbleQueue, copyForFinish, BUBBLE_MS } from "./bubbles.js";

test("copyForFinish rotates deterministically by seq and includes the name", () => {
  assert.equal(copyForFinish("proj", 3), "proj is done! ✨");        // 3 % 3 == 0
  assert.equal(copyForFinish("proj", 1), "all wrapped up in proj 💫"); // 1 % 3 == 1
  assert.equal(copyForFinish("proj", 2), "proj's ready for you~");     // 2 % 3 == 2
});

test("copyForFinish falls back to 'a session' for blank names", () => {
  assert.equal(copyForFinish("", 3), "a session is done! ✨");
  assert.equal(copyForFinish("   ", 3), "a session is done! ✨");
});

test("first ingest primes: pre-existing finishes never bubble", () => {
  const q = new BubbleQueue();
  q.ingest({ finishSeq: 2, lastFinished: "old" }); // baseline at mount
  assert.equal(q.tick(1000), null);
});

test("ingest enqueues once per seq increment after priming", () => {
  const q = new BubbleQueue();
  q.ingest({ finishSeq: 0, lastFinished: "" });     // prime
  q.ingest({ finishSeq: 1, lastFinished: "alpha" });
  q.ingest({ finishSeq: 1, lastFinished: "alpha" }); // same seq (re-push) -> no dupe
  assert.equal(q.tick(0), copyForFinish("alpha", 1));
  // Only one bubble was queued: it expires and nothing follows.
  assert.equal(q.tick(BUBBLE_MS), null);
});

test("head expires after BUBBLE_MS, then the next queued line shows", () => {
  const q = new BubbleQueue();
  q.ingest({ finishSeq: 0, lastFinished: "" }); // prime
  q.ingest({ finishSeq: 1, lastFinished: "a" });
  q.ingest({ finishSeq: 2, lastFinished: "b" });
  assert.equal(q.tick(0), copyForFinish("a", 1));
  assert.equal(q.tick(BUBBLE_MS - 1), copyForFinish("a", 1)); // still within window
  assert.equal(q.tick(BUBBLE_MS), copyForFinish("b", 2));     // a expired, b shows
  assert.equal(q.tick(BUBBLE_MS * 2), null);                  // b expired, empty
});

test("dismiss drops the head early and reveals the next", () => {
  const q = new BubbleQueue();
  q.ingest({ finishSeq: 0, lastFinished: "" }); // prime
  q.ingest({ finishSeq: 1, lastFinished: "a" });
  q.ingest({ finishSeq: 2, lastFinished: "b" });
  assert.equal(q.tick(0), copyForFinish("a", 1));
  q.dismiss(10);
  assert.equal(q.tick(10), copyForFinish("b", 2));
});

test("the dwell is injectable, and the default is unchanged for the overlay", () => {
  // The desktop overlay constructs BubbleQueue with no argument and must keep
  // its long-standing timing; the sidebar host passes a user-tunable value.
  const dflt = new BubbleQueue();
  assert.equal(dflt.ttlMs, BUBBLE_MS);

  const q = new BubbleQueue(9000);
  q.ingest({ finishSeq: 0 });
  q.ingest({ finishSeq: 1, lastFinished: "api" });
  assert.ok(q.tick(0));
  assert.ok(q.tick(BUBBLE_MS + 1), "must still be showing past the old default");
  assert.equal(q.tick(9001), null, "and gone once its own ttl elapses");
});

test("retiming a live queue applies to the line already on screen", () => {
  // Settings drags the slider mid-notice; the shown line should retime rather
  // than only the next one.
  const q = new BubbleQueue(10000);
  q.ingest({ finishSeq: 0 });
  q.ingest({ finishSeq: 1, lastFinished: "api" });
  assert.ok(q.tick(0));
  q.ttlMs = 2000;
  assert.equal(q.tick(2001), null);
});
