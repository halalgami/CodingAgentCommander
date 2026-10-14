import { test } from "node:test";
import assert from "node:assert/strict";
import { autoTitle, gaveUpText } from "./autocont.js";

test("toggle title marks the default and the version gate", () => {
  const base = "Auto-continue: when this model stops right after announcing a step, tell it to carry on";
  assert.equal(autoTitle({ autoContinueReady: true, autoContinueSet: true }), base);
  assert.equal(autoTitle({ autoContinueReady: true, autoContinueSet: false }), base + " (default)");
  assert.equal(autoTitle({ autoContinueReady: false }), "Needs Claude Code 2.1.210 or newer");
});

test("gave-up toast text", () => {
  assert.equal(gaveUpText({ model: "Ollama · glm-5.3", why: "stuck" }), "Ollama · glm-5.3 seems stuck — over to you");
  assert.equal(gaveUpText({ model: "Ollama · glm-5.3", why: "cap" }), "Ollama · glm-5.3 auto-continued 20 times — over to you");
});
