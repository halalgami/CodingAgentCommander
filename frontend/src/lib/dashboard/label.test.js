import { test } from "node:test";
import assert from "node:assert/strict";
import { labelOf } from "./label.js";

test("project is the headline, the session title is the task", () => {
  assert.deepEqual(labelOf({ project: "AlAjeerBrand", title: "Brand stationery and print kit", activity: "finished" }),
    { project: "AlAjeerBrand", task: "Brand stationery and print kit", titled: true });
});

test("an untitled session or a title that repeats the project shows the activity instead", () => {
  const busy = { project: "expert_physio_platform", activity: "tool", tool: "Edit", target: "app.go" };
  for (const title of ["expert_physio_platform", "Untitled session", "", undefined]) {
    const l = labelOf({ ...busy, title });
    assert.equal(l.project, "expert_physio_platform");
    assert.notEqual(l.task, "expert_physio_platform");
    assert.match(l.task, /app\.go/);
  }
});
