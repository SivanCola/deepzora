#!/usr/bin/env node
// Shared cleanup entry point for create-dmg and mounted-DMG acceptance runs.
import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { cleanupStaging, detachImage, macOSDriver } from "./desktop-dmg-lifecycle.mjs";

const [mode, ...args] = process.argv.slice(2);
try {
  if (mode === "hdiutil") {
    const real = process.env.DEEPZORA_HDIUTIL;
    if (!real || !process.env.DEEPZORA_DMG_STAGING) throw new Error("Missing scoped hdiutil configuration");
    if (["detach", "unmount", "eject"].includes(args[0])) {
      if (args.length !== 2 || args[1].startsWith("-")) throw new Error("Only a normal, guarded DMG detach is allowed");
      await detachImage(args[1], { appName: process.env.DEEPZORA_DMG_APP_NAME, staging: process.env.DEEPZORA_DMG_STAGING }, macOSDriver(real));
    } else {
      const result = spawnSync(real, args, { stdio: "inherit" });
      if (result.error) throw result.error;
      process.exitCode = result.status ?? 1;
    }
  } else if (mode === "cleanup-staging" && args.length === 2) {
    await cleanupStaging(args[0], args[1], macOSDriver(process.env.DEEPZORA_HDIUTIL));
  } else if (mode === "detach" && args.length >= 1) {
    await detachImage(args[0], { extraApps: args.slice(1) });
  } else {
    throw new Error("usage: desktop-dmg.mjs detach <mount-or-device> [copied-app ...] | cleanup-staging <directory> <app-name>");
  }
} catch (error) {
  console.error(error.message);
  // create-dmg tolerates some errors; an unsafe/forced cleanup must never be
  // mistaken for a mere Finder customization error by the publication step.
  if (process.env.DEEPZORA_DMG_FAILURE) writeFileSync(process.env.DEEPZORA_DMG_FAILURE, `${error.message}\n`);
  process.exitCode = 1;
}
