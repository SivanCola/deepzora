import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./package-desktop-dmg.sh", import.meta.url));
for (const scenario of ["no-output", "invalid-output", "customization-error", "success", "guarded-detach", "busy-detach", "ignored-detach-error", "failed-while-mounted"]) {
  test(`fresh DMG publication: ${scenario}`, t => {
    const root = mkdtempSync(join(tmpdir(), "deepzora-dmg-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const app = join(root, "Deepzora.app");
    const bin = join(root, "bin");
    const dist = join(root, "dist");
    for (const dir of [app, bin, dist]) mkdirSync(dir);
    writeFileSync(join(app, "build.json"), "current-build");
    const output = join(dist, "Deepzora.dmg");
    writeFileSync(output, "old-build");
    writeFileSync(join(bin, "create-dmg"), `#!/usr/bin/env bash
set -eu
while [ "$#" -gt 2 ]; do shift; done
[ "$1" != "$OUTPUT" ]
[ ! -e "$1" ]
[ "$(cat "$2/Deepzora.app/build.json")" = current-build ]
case "$SCENARIO" in
  no-output) exit 1 ;;
  invalid-output) printf invalid > "$1" ;;
  customization-error) printf current-build > "$1"; exit 1 ;;
  success) printf current-build > "$1" ;;
  guarded-detach|busy-detach|ignored-detach-error|failed-while-mounted)
    printf '%s' "$DEEPZORA_DMG_STAGING/rw.candidate.dmg" > "$STATE"
    printf backing-image > "$DEEPZORA_DMG_STAGING/rw.candidate.dmg"
    [ "$SCENARIO" != failed-while-mounted ] || exit 1
    hdiutil detach /dev/disk99 || { [ "$SCENARIO" = ignored-detach-error ] || exit 1; }
    printf current-build > "$1"
    ;;
esac
`, { mode: 0o755 });
    writeFileSync(join(bin, "hdiutil"), `#!/usr/bin/env bash
set -eu
if [ "$1" = info ]; then
  if [ -f "$STATE" ]; then
    printf '{"images":[{"image-path":"%s","system-entities":[{"dev-entry":"/dev/disk99","mount-point":"%s/mounted volume"}]}]}' "$(cat "$STATE")" "$TEST_ROOT"
  else
    printf '{"images":[]}'
  fi
  exit 0
fi
if [ "$1" = detach ]; then
  printf '%s\\n' "$*" >> "$TEST_ROOT/detaches"
  case "$SCENARIO" in
    busy-detach|ignored-detach-error) echo 'Resource busy' >&2; exit 16 ;;
  esac
  rm "$STATE"
  exit 0
fi
[ "$1" = verify ]
[ "$(cat "$2")" = current-build ]
`, { mode: 0o755 });
    writeFileSync(join(bin, "plutil"), '#!/usr/bin/env bash\ncat\n', { mode: 0o755 });
    const result = spawnSync("bash", [script, app, output, "Deepzora"], {
      encoding: "utf8",
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, OUTPUT: output, SCENARIO: scenario, STATE: join(root, "mounted"), TEST_ROOT: root },
    });
    const succeeds = ["customization-error", "success", "guarded-detach"].includes(scenario);
    assert.equal(result.status === 0, succeeds, result.stderr);
    assert.equal(readFileSync(output, "utf8"), succeeds ? "current-build" : "old-build");
    if (["busy-detach", "ignored-detach-error"].includes(scenario)) {
      const staging = readdirSync(dist).find(name => name.startsWith(".desktop-dmg."));
      assert.ok(staging, "unsafe cleanup must retain staging");
      assert.equal(readFileSync(join(dist, staging, "rw.candidate.dmg"), "utf8"), "backing-image");
      assert.match(result.stderr, /retained evidence/);
      assert.doesNotMatch(readFileSync(join(root, "detaches"), "utf8"), /-force/);
    } else {
      assert.deepEqual(readdirSync(dist), ["Deepzora.dmg"]);
    }
  });
}
