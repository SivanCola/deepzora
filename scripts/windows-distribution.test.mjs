import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { windowsInstallerOnly, windowsCompatibilityArchive, verifyWindowsCompatibilityArchive } from "./windows-distribution.mjs";

test("Windows distribution changes at v0.0.8 without changing historical releases", () => {
  for (const version of ["0.0.1", "v0.0.6", "v0.0.7"]) assert.equal(windowsInstallerOnly(version), false);
  for (const version of ["v0.0.8", "0.0.8-preview.1", "0.1.0", "1.0.0"]) assert.equal(windowsInstallerOnly(version), true);
  for (const version of ["", "latest", "../0.0.8", "0.0.08"]) assert.throws(() => windowsInstallerOnly(version));
  assert.throws(() => windowsCompatibilityArchive("0.0.7", "amd64"), /historical/);
});

test("compatibility ZIP contains only a migration notice for the matching signed installer", t => {
  const root = mkdtempSync(path.join(tmpdir(), "deepzora-compatibility-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const arch of ["amd64", "arm64"]) {
    const bytes = windowsCompatibilityArchive("0.0.8", arch);
    assert.ok(bytes.length < 2048, "compatibility files must not contain an application payload");
    const file = path.join(root, arch + ".zip");
    writeFileSync(file, bytes);
    assert.equal(execFileSync("unzip", ["-Z1", file], { encoding: "utf8" }).trim(), "WINDOWS-INSTALLER-ONLY.txt");
    const text = execFileSync("unzip", ["-p", file], { encoding: "utf8" });
    assert.ok(text.includes("仅供旧版更新清单兼容"));
    assert.ok(text.includes(`/desktop-v0.0.8/Deepzora-0.0.8-Windows-${arch === "amd64" ? "x64" : "arm64"}-Setup.exe`));
    assert.doesNotThrow(() => verifyWindowsCompatibilityArchive(bytes, "0.0.8", arch));
    assert.throws(() => verifyWindowsCompatibilityArchive(bytes, "0.0.9", arch), /exact/);
    assert.throws(() => verifyWindowsCompatibilityArchive(bytes, "0.0.8", arch === "amd64" ? "arm64" : "amd64"), /exact/);
    assert.throws(() => verifyWindowsCompatibilityArchive(Buffer.concat([bytes, Buffer.from("MZ")]), "0.0.8", arch), /exact/);
  }
});
