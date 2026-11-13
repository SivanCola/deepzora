import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function runPackageTool(command, args, options) {
  if (process.platform !== "win32") return execFileSync(command, args, options);
  // Windows package managers are .cmd shims. Only the fixed tokens below
  // enter cmd.exe; the potentially space-containing project path uses cwd.
  const tokens = [command, ...args];
  if (tokens.some(token => !/^[A-Za-z0-9@./=-]+$/.test(token))) {
    throw new Error("Web UI package-manager invocation contains an unsafe token");
  }
  return execFileSync(process.env.ComSpec || "cmd.exe", ["/d", "/s", "/c", tokens.join(" ")], options);
}

function pnpm(args) {
  try { runPackageTool("corepack", ["--version"], { stdio: "ignore" }); }
  catch { return runPackageTool("pnpm", args, { cwd: ROOT, stdio: "inherit" }); }
  return runPackageTool("corepack", ["pnpm@10.34.5", ...args], { cwd: ROOT, stdio: "inherit" });
}

// The Go service embeds the same browser-safe UI used by the desktop shell.
// Build it before every distributable CLI; Node is only a build dependency.
export function buildWebUI() {
  pnpm(["--dir", "desktop", "install", "--frozen-lockfile"]);
  pnpm(["--dir", "desktop/frontend", "build"]);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) buildWebUI();
