import assert from "node:assert/strict";
import test from "node:test";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync, realpathSync } from "node:fs";
import { execFileSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { execute, packageBatches, testBatches, packageResults, runWindowsRoot } from "./ci-windows-root.mjs";

test("CLI and durable control packages run alone without removing or repeating any package", () => {
  const packages = ["deepzora/internal/agent", "deepzora/internal/cli", "deepzora/internal/control", "deepzora/internal/session"];
  const groups = testBatches(packages);
  assert.deepEqual(groups.slice(0, 2), [["deepzora/internal/cli"], ["deepzora/internal/control"]]);
  assert.deepEqual(groups.flat().sort(), packages);
  assert.equal(groups.slice(2).flat().includes("deepzora/internal/control"), false);
  assert.deepEqual(testBatches(["deepzora/internal/control"]), [["deepzora/internal/control"]]);
});

test("long package arguments are bounded without losing or duplicating packages", () => {
  const packages = Array.from({ length: 500 }, (_, i) => `example.test/${"deep/".repeat(20)}p${i}`);
  const groups = packageBatches(packages);
  assert.ok(groups.length > 1);
  assert.deepEqual(groups.flat(), packages);
  assert.ok(groups.every(group => group.join(" ").length + 100 <= 12_000));
  assert.throws(() => packageBatches(["p".repeat(12_000)]));
});

test("package terminal verification rejects missing, duplicate, failed and helper test rows", () => {
  const events = [{ Package: "a", Action: "pass" }, { Package: "b", Action: "skip" }, { Package: "a", Test: "TestHelper", Action: "pass" }];
  assert.equal(packageResults(events, ["a", "b"]).length, 2);
  assert.throws(() => packageResults(events.slice(1), ["a", "b"]));
  assert.throws(() => packageResults([...events, events[0]], ["a", "b"]));
  assert.throws(() => packageResults([{ Package: "a", Action: "fail" }], ["a"]));
});

test("process runner preserves failures, enforces a deadline and accepts spaced working paths", async t => {
  const cwd = mkdtempSync(path.join(os.tmpdir(), "ci windows root runner "));
  t.after(() => rmSync(cwd, { recursive: true, force: true }));
  const result = await execute(process.execPath, ["-e", "process.stdout.write(process.cwd())"], { cwd, deadline: Date.now() + 5000 });
  assert.equal(result.output, realpathSync(cwd));
  await assert.rejects(execute(process.execPath, ["-e", "process.exit(7)"], { cwd, deadline: Date.now() + 5000 }), /exited 7/);
  await assert.rejects(execute(process.execPath, ["-e", "setInterval(()=>{},1000)"], { cwd, deadline: Date.now() + 100 }), /deadline/);
  await assert.rejects(execute(process.execPath, ["-e", "0"], { cwd, deadline: Date.now() - 1 }), /deadline/);
  await assert.rejects(execute("definitely-missing-ci-fixture-program", [], { cwd, deadline: Date.now() + 1000 }), /ENOENT/);
  const pending = execute(process.execPath, ["-e", "setInterval(()=>{},1000)"], { cwd, deadline: Date.now() + 5000 });
  const cancellation = setTimeout(() => process.emit("SIGTERM"), 100);
  await assert.rejects(pending, /cancelled/);
  clearTimeout(cancellation);
});

test("real Go runner preserves complete package builds, test execution, no-test packages and failures", { timeout: 180_000 }, async t => {
  const root = mkdtempSync(path.join(os.tmpdir(), "windows root fixture "));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  writeFileSync(path.join(root, "go.mod"), "module deepzora\n\ngo 1.26\n");
  writeFileSync(path.join(root, "main.go"), "package main\nfunc main() {}\n");
  for (const name of ["a", "b", "internal/cli", "internal/control"]) {
    mkdirSync(path.join(root, name), { recursive: true });
    writeFileSync(path.join(root, name, "value.go"), "package value\nfunc Value() int { return 42 }\n");
    writeFileSync(path.join(root, name, "value_test.go"), 'package value\nimport ("testing"; "os"; "slices")\nfunc TestValue(t *testing.T) { if Value()!=42 { t.Fatal("wrong value") }; if !slices.Contains(os.Args,"-test.count=1") { t.Fatal("test-result caching was not disabled") } }\n');
  }
  mkdirSync(path.join(root, "testonly"));
  writeFileSync(path.join(root, "testonly", "only_test.go"), 'package testonly\nimport "testing"\nfunc TestOnly(t *testing.T) {}\n');
  execFileSync("git", ["init", "-q"], { cwd: root });
  execFileSync("git", ["-c", "user.name=CI Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture", "--allow-empty"], { cwd: root });

  const report = await runWindowsRoot({ directory: root, output: path.join(root, "whole") });
  assert.equal(report.completed.length, 6);
  assert.ok(report.completed.some(r => r.package === "deepzora/testonly" && r.result === "pass"));
  assert.ok(report.completed.some(r => r.package === "deepzora" && r.result === "skip"));
  assert.ok(!report.buildInventory.includes("deepzora/testonly"));
  assert.deepEqual(report.testBatches.slice(0, 2), [["deepzora/internal/cli"], ["deepzora/internal/control"]]);
  const events = readFileSync(path.join(root, "whole", "tests.jsonl"), "utf8").trim().split("\n").map(JSON.parse);
  const cliEnd = events.findIndex(e => e.Package === "deepzora/internal/cli" && !e.Test && e.Action === "pass");
  const controlStart = events.findIndex(e => e.Package === "deepzora/internal/control" && e.Action === "start");
  const controlEnd = events.findIndex(e => e.Package === "deepzora/internal/control" && !e.Test && e.Action === "pass");
  assert.ok(cliEnd >= 0 && cliEnd < controlStart && controlStart < controlEnd);
  assert.ok(events.findIndex(e => e.Action === "start" && !["deepzora/internal/cli", "deepzora/internal/control"].includes(e.Package)) > controlEnd);

  writeFileSync(path.join(root, "a", "value.go"), "package value\nfunc Value() int { return 0 }\n");
  await assert.rejects(runWindowsRoot({ directory: root, output: path.join(root, "failure") }));
  assert.equal(JSON.parse(readFileSync(path.join(root, "failure", "report.json"))).status, "failure");
});
