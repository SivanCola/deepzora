import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

// Legacy clients validate the complete v1 download matrix, including two ZIP
// names they never use for Windows updates (platforms routes to Setup.exe).
export function windowsInstallerOnly(version) {
  assert.match(version ?? "", /^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$/);
  const [major, minor, patch] = version.replace(/^v/, "").split(/[.-]/).map(Number);
  return major > 0 || minor > 0 || patch >= 8;
}

function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

// One deterministic, uncompressed UTF-8 text entry. There are no executables,
// scripts, redirects or old application payloads in this compatibility file.
export function windowsCompatibilityArchive(version, arch) {
  assert.ok(windowsInstallerOnly(version), "historical portable releases must retain their real payload");
  assert.ok(["amd64", "arm64"].includes(arch), "unsupported Windows architecture");
  version = version.replace(/^v/, "");
  const cpu = arch === "amd64" ? "x64" : "arm64";
  const url = `https://github.com/SivanCola/deepzora/releases/download/desktop-v${version}/Deepzora-${version}-Windows-${cpu}-Setup.exe`;
  const text = Buffer.from([
    "Deepzora Windows installer only / Windows 版本仅提供安装包",
    "",
    "The portable application was discontinued starting with v0.0.8.",
    "This small ZIP exists only for the manifest validation in v0.0.7 and older updaters.",
    "It contains no application. Download and run the signed installer below.",
    "Existing Windows installations, including portable installations, update using Setup.exe.",
    "从 v0.0.8 起不再提供便携应用。此 ZIP 仅供旧版更新清单兼容，不包含程序。",
    "请下载并运行下方的已签名安装包。旧版应用的自动更新仍通过安装包完成。",
    "", url, "",
  ].join("\r\n"), "utf8");
  const name = Buffer.from("WINDOWS-INSTALLER-ONLY.txt");
  const header = Buffer.alloc(30);
  header.writeUInt32LE(0x04034b50); header.writeUInt16LE(20, 4);
  header.writeUInt16LE(0x800, 6); header.writeUInt16LE(0x21, 12);
  header.writeUInt32LE(crc32(text), 14); header.writeUInt32LE(text.length, 18);
  header.writeUInt32LE(text.length, 22); header.writeUInt16LE(name.length, 26);
  const central = Buffer.alloc(46);
  central.writeUInt32LE(0x02014b50); central.writeUInt16LE(20, 4);
  header.copy(central, 6, 4, 30);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50); end.writeUInt16LE(1, 8); end.writeUInt16LE(1, 10);
  end.writeUInt32LE(central.length + name.length, 12);
  end.writeUInt32LE(header.length + name.length + text.length, 16);
  return Buffer.concat([header, name, text, central, name, end]);
}

export function verifyWindowsCompatibilityArchive(bytes, version, arch) {
  assert.ok(Buffer.from(bytes).equals(windowsCompatibilityArchive(version, arch)),
    "Windows compatibility ZIP must contain only the exact installer migration notice");
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const [command, version, arch, file] = process.argv.slice(2);
  if (command === "mode") console.log(windowsInstallerOnly(version) ? "installer-only" : "legacy-portable");
  else if (command === "create") writeFileSync(file, windowsCompatibilityArchive(version, arch));
  else if (command === "verify") verifyWindowsCompatibilityArchive(readFileSync(file), version, arch);
  else throw new Error("usage: windows-distribution.mjs mode VERSION | create|verify VERSION ARCH ZIP");
}
