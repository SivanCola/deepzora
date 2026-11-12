import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function pnpm(args) {
  try { execFileSync("corepack", ["--version"], { stdio: "ignore" }); }
  catch { return execFileSync("pnpm", args, { cwd: ROOT, stdio: "inherit" }); }
  return execFileSync("corepack", ["pnpm@10.34.5", ...args], { cwd: ROOT, stdio: "inherit" });
}

// The Go service embeds the same browser-safe UI used by the desktop shell.
// Build it before every distributable CLI; Node is only a build dependency.
export function buildWebUI() {
  pnpm(["--dir", "desktop", "install", "--frozen-lockfile"]);
  pnpm(["--dir", "desktop/frontend", "build"]);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) buildWebUI();
