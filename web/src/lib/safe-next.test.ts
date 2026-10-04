// Run with `npm test` (Node's built-in test runner, no extra dependency).
import assert from "node:assert/strict";
import { test } from "node:test";
import { safeNext } from "./safe-next.ts";

test("keeps same-site paths", () => {
  assert.equal(safeNext("/problems/two-sum"), "/problems/two-sum");
  assert.equal(safeNext("/problems?page=2"), "/problems?page=2");
});

test("refuses anything that can leave the site", () => {
  for (const bad of [
    null,
    "",
    "https://evil.example",
    "//evil.example",
    "/\\evil.example",
    "/\\/evil.example",
    "/\t/evil.example",
    "/\n/evil.example",
    "evil.example",
    "javascript:alert(1)",
  ]) {
    assert.equal(safeNext(bad), "/problems", `input ${JSON.stringify(bad)}`);
  }
});
