// Opt-in macOS integration: real DMG, real executable mappings, no product data.
// DEEPZORA_TEST_NATIVE_DMG=1 node --test scripts/desktop-dmg-native.test.mjs
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { copyFileSync, mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { cleanupStaging, detachImage, macOSDriver } from "./desktop-dmg-lifecycle.mjs";

test("native DMG cleanup verifies actual shell/backend exit and preserves another installation", {
  skip: process.platform !== "darwin" || process.env.DEEPZORA_TEST_NATIVE_DMG !== "1",
  timeout: 180_000,
}, async () => {
  const root = mkdtempSync(join(tmpdir(), "deepzora dmg native "));
  const source = join(root, "input");
  const mount = join(root, "mounted");
  const image = join(root, "fixture.dmg");
  const shellRelative = "Deepzora.app/Contents/MacOS/Deepzora";
  const serviceRelative = "Deepzora.app/Contents/Resources/service/deepzora-desktop";
  const children = [];
  const driver = macOSDriver();
  let mounted = false;
  try {
    mkdirSync(join(source, "Deepzora.app/Contents/MacOS"), { recursive: true });
    mkdirSync(join(source, "Deepzora.app/Contents/Resources/service"), { recursive: true });
    mkdirSync(mount);
    const c = join(root, "fixture.c");
    writeFileSync(c, '#include <signal.h>\n#include <stdio.h>\n#include <unistd.h>\nint main(int argc, char **argv) { if (argc > 1) signal(SIGTERM, SIG_IGN); puts("ready"); fflush(stdout); for (;;) pause(); }\n');
    execFileSync("cc", [c, "-o", join(source, shellRelative)]);
    copyFileSync(join(source, shellRelative), join(source, serviceRelative));
    // Exercise the real create-dmg/hdiutil interception too, omitting only
    // Finder's visual customization so this test needs no GUI automation.
    const createDMG = execFileSync("/usr/bin/which", ["create-dmg"], { encoding: "utf8" }).trim();
    const bin = join(root, "bin");
    mkdirSync(bin);
    writeFileSync(join(bin, "create-dmg"), '#!/usr/bin/env bash\nexec "$DEEPZORA_NATIVE_CREATE_DMG" --skip-jenkins "$@"\n', { mode: 0o755 });
    execFileSync("bash", [fileURLToPath(new URL("./package-desktop-dmg.sh", import.meta.url)), join(source, "Deepzora.app"), image, "Deepzora"], {
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, DEEPZORA_NATIVE_CREATE_DMG: createDMG },
      stdio: "inherit",
    });
    execFileSync("/usr/bin/hdiutil", ["attach", "-readonly", "-nobrowse", "-mountpoint", mount, image]);
    mounted = true;
    async function launch(executable, args = []) {
      const child = spawn(executable, args, { stdio: ["ignore", "pipe", "pipe"] });
      children.push(child);
      child.finished = once(child, "exit");
      await once(child.stdout, "data");
      return child;
    }
    const unrelated = await launch(join(source, shellRelative));
    const shell = await launch(join(mount, shellRelative));
    const backend = await launch(join(mount, serviceRelative), ["ignore-term"]);
    const originalDetach = driver.detach;
    driver.detach = target => {
      assert.ok(!driver.processes().some(p => p.pid === shell.pid || p.pid === backend.pid), "no app executable may remain when hdiutil runs");
      originalDetach(target);
    };
    await assert.rejects(detachImage(mount, {}, driver), /required SIGKILL/);
    mounted = false;
    await Promise.all([shell.finished, backend.finished]);
    assert.equal(shell.signalCode, "SIGTERM");
    assert.equal(backend.signalCode, "SIGKILL");
    assert.equal(unrelated.exitCode, null);
    assert.equal(unrelated.signalCode, null);
    assert.ok(!driver.images().some(item => item["image-path"] === image));
  } finally {
    for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL");
    await Promise.all(children.map(child => child.finished));
    if (mounted) execFileSync("/usr/bin/hdiutil", ["detach", mount]);
    // The package step itself may have failed while an interstitial image
    // remained mounted. Never remove its backing files in test teardown.
    await cleanupStaging(root, "Deepzora");
    rmSync(root, { recursive: true, force: true });
  }
});
