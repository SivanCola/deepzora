import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./build-web-ui.mjs", import.meta.url));

function fixture(t, { corepackMissing = false, buildFails = false } = {}) {
  const directory = mkdtempSync(path.join(tmpdir(), "deepzora web tools "));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const record = path.join(directory, "commands.jsonl");
  const runner = path.join(directory, "package-tool.cjs");
  writeFileSync(runner, `
const fs = require("node:fs");
const [tool, ...args] = process.argv.slice(2);
fs.appendFileSync(process.env.WEB_UI_TOOL_RECORD, JSON.stringify({tool,args}) + "\\n");
if (tool === "corepack" && process.env.WEB_UI_NO_COREPACK === "1") process.exit(1);
if (args.at(-1) === "build" && process.env.WEB_UI_BUILD_FAILS === "1") process.exit(23);
console.log("10.34.5");
`);
  const quote = value => `'${value.replaceAll("'", `'\\''`)}'`;
  for (const tool of ["corepack", "pnpm"]) {
    const windows = process.platform === "win32";
    writeFileSync(path.join(directory, tool + (windows ? ".cmd" : "")), windows
      ? `@echo off\r\n"${process.execPath}" "${runner}" ${tool} %*\r\n`
      : `#!/bin/sh\nexec ${quote(process.execPath)} ${quote(runner)} ${tool} "$@"\n`, { mode: 0o755 });
  }
  // Windows treats PATH case-insensitively; do not pass duplicate Path/PATH
  // keys that could cause Node to select the real package manager instead.
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => key.toLowerCase() !== "path"));
  env.PATH = directory + path.delimiter + (process.env.PATH ?? "");
  env.WEB_UI_TOOL_RECORD = record;
  env.WEB_UI_NO_COREPACK = corepackMissing ? "1" : "0";
  env.WEB_UI_BUILD_FAILS = buildFails ? "1" : "0";
  const result = spawnSync(process.execPath, [script], { env, encoding: "utf8", timeout: 30_000 });
  assert.ifError(result.error);
  const commands = readFileSync(record, "utf8").trim().split("\n").map(line => JSON.parse(line));
  return { result, commands };
}

test("Web UI bootstrap executes Corepack shims from a tool path containing spaces", t => {
  const { result, commands } = fixture(t);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(commands, [
    { tool: "corepack", args: ["--version"] },
    { tool: "corepack", args: ["pnpm@10.34.5", "--dir", "desktop", "install", "--frozen-lockfile"] },
    { tool: "corepack", args: ["--version"] },
    { tool: "corepack", args: ["pnpm@10.34.5", "--dir", "desktop/frontend", "build"] },
  ]);
});

test("Web UI bootstrap uses the installed pnpm shim when Corepack is unavailable", t => {
  const { result, commands } = fixture(t, { corepackMissing: true });
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(commands.filter(row => row.tool === "pnpm").map(row => row.args), [
    ["--dir", "desktop", "install", "--frozen-lockfile"], ["--dir", "desktop/frontend", "build"],
  ]);
});

test("Web UI build failure propagates without falling back to another package manager", t => {
  const { result, commands } = fixture(t, { buildFails: true });
  assert.notEqual(result.status, 0);
  assert.equal(commands.some(row => row.tool === "pnpm"), false);
  assert.equal(commands.at(-1).args.at(-1), "build");
});
