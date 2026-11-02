#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { mkdirSync, openSync, writeSync, closeSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Timeout/cancellation applies to the process tree, including helper executables.
export function execute(command, args, { cwd, deadline, log, json = false, env = process.env } = {}) {
  return new Promise((resolve, reject) => {
    const remaining = deadline - Date.now();
    if (remaining <= 0) return reject(new Error("shared test deadline exceeded"));
    const child = spawn(command, args, { cwd, env, shell: false, detached: process.platform !== "win32", stdio: ["ignore", "pipe", "pipe"] });
    const events = [];
    let partial = "", output = "", aborted, settled = false;
    const terminate = reason => {
      if (settled || aborted) return;
      aborted = new Error(reason);
      if (process.platform === "win32") {
        if (child.pid) spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore", shell: false });
      } else if (child.pid) {
        try { process.kill(-child.pid, "SIGKILL"); } catch { child.kill("SIGKILL"); }
      }
    };
    const onSignal = () => terminate("test execution cancelled");
    process.on("SIGTERM", onSignal); process.on("SIGINT", onSignal);
    const timer = setTimeout(() => terminate("shared test deadline exceeded"), remaining);
    const progress = setInterval(() => console.log(`test process active (${events.filter(event => event.Action === "pass" && event.Test && !event.Test.includes("/")).length} top-level tests completed)`), 30_000);
    function consume(line) {
      if (!json) return;
      let event;
      try { event = JSON.parse(line); } catch { return; }
      // Output events can contain helper subprocess text; only the parent
      // test2json framing produces authoritative completion events.
      if (event.Action !== "output") events.push(event);
      if (event.Action === "fail") console.error(`FAIL ${event.Package ?? ""} ${event.Test ?? ""}`);
    }
    child.stdout.setEncoding("utf8"); child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => {
      if (log !== undefined) writeSync(log, chunk);
      if (!json) output += chunk;
      else {
        partial += chunk;
        const lines = partial.split("\n"); partial = lines.pop();
        for (const line of lines) consume(line);
      }
    });
    child.stderr.on("data", chunk => { if (log !== undefined) writeSync(log, chunk); if (!json) output += chunk; });
    function finish(error, code) {
      if (settled) return;
      settled = true; clearTimeout(timer); clearInterval(progress);
      process.off("SIGTERM", onSignal); process.off("SIGINT", onSignal);
      if (partial) consume(partial);
      if (error || aborted || code !== 0) {
        const failure = error ?? aborted ?? new Error(`${command} exited ${code}; see raw test log`);
        failure.events = events;
        reject(failure);
      } else resolve({ output, events });
    }
    child.on("error", error => finish(error));
    child.on("close", code => finish(null, code));
  });
}

export function packageResults(events, expected) {
  const rows = events.filter(e => !e.Test && ["pass", "fail", "skip"].includes(e.Action))
    .map(e => ({ package: e.Package, result: e.Action })).sort((a, b) => a.package.localeCompare(b.package));
  assert.deepEqual(rows.map(r => r.package).sort(), expected, "missing, duplicate or unexpected package result");
  assert.ok(rows.every(r => r.result !== "fail"), "package failed");
  return rows;
}

export function packageBatches(packages, limit = 12_000) {
  const groups = []; let group = [], size = 100;
  for (const pkg of packages) {
    assert.ok(pkg.length + 100 < limit, "package name exceeds command budget");
    if (size + pkg.length + 1 > limit) { groups.push(group); group = []; size = 100; }
    group.push(pkg); size += pkg.length + 1;
  }
  if (group.length) groups.push(group);
  return groups;
}

export function testBatches(packages) {
  // CLI's bounded shell tests and control's durable turns run before competing
  // package writers. Keep each package whole.
  const isolated = ["deepzora/internal/cli", "deepzora/internal/control"].filter(p => packages.includes(p));
  return [...isolated.map(p => [p]), ...packageBatches(packages.filter(p => !isolated.includes(p)))];
}

export async function runWindowsRoot({ directory = ".", output }) {
  const cwd = path.resolve(directory), destination = path.resolve(output);
  mkdirSync(destination, { recursive: true });
  const query = args => execFileSync(args[0], args.slice(1), { cwd, encoding: "utf8", timeout: 60_000 }).trim();
  const r = { schemaVersion: 3, status: "failure", buildCompleted: false, completed: [], timings: {},
    sourceSHA: query(["git", "rev-parse", "HEAD"]), runId: process.env.GITHUB_RUN_ID ?? "0", attempt: process.env.GITHUB_RUN_ATTEMPT ?? "1",
    platform: process.platform, arch: process.arch, goVersion: query(["go", "version"]) };
  const log = openSync(path.join(destination, "tests.jsonl"), "w");
  // The workflow still has its existing 35-minute limit, including setup and
  // both PowerShell checks. Every Go package retains its 15-minute test limit.
  const deadline = Date.now() + 30 * 60_000;
  try {
    const packages = query(["go", "list", "-f", "{{.ImportPath}} {{if or .GoFiles .CgoFiles}}build{{else}}test-only{{end}}", "./..."])
      .split(/\r?\n/).map(line => line.split(" "));
    r.inventory = packages.map(([name]) => name).sort();
    assert.ok(r.inventory.length, "empty package inventory");
    // go build ./... excludes packages containing only test files. Explicit
    // package arguments do not, so preserve that behavior without dropping
    // those packages from go test or the coverage check.
    r.buildInventory = packages.filter(([, kind]) => kind === "build").map(([name]) => name).sort();
    let start = Date.now();
    r.testBatches = testBatches(r.inventory);
    for (const group of packageBatches(r.buildInventory)) await execute("go", ["build", ...group], { cwd, log, deadline });
    r.timings.buildMs = Date.now() - start;
    r.buildCompleted = true;
    start = Date.now();
    const events = [];
    // -count=1 disables test-result caching, whose input hashing cost minutes on
    // Windows; -p=2 keeps filesystem-heavy packages from crowding process fixtures.
    for (const group of r.testBatches) {
      const result = await execute("go", ["test", "-json", "-timeout=15m", "-p=2", "-count=1", ...group], { cwd, log, deadline, json: true });
      events.push(...result.events);
    }
    r.timings.testMs = Date.now() - start;
    r.completed = packageResults(events, r.inventory);
    r.status = "success";
  } catch (error) {
    r.error = error.message;
    r.completed = (error.events ?? []).filter(e => !e.Test && ["pass", "fail", "skip"].includes(e.Action))
      .map(e => ({ package: e.Package, result: e.Action }));
    throw error;
  } finally {
    closeSync(log);
    writeFileSync(path.join(destination, "report.json"), JSON.stringify(r, null, 2) + "\n");
  }
  console.log(`Windows root: ${r.completed.length} packages built and tested`);
  return r;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  try {
    const args = process.argv.slice(2);
    assert.equal(args.length, 1, "expected OUTPUT");
    await runWindowsRoot({ output: args[0] });
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
