// Run: tsx src/__tests__/add-project-entries.test.ts
// Source-contract test: the add-project entry points offer only local
// project actions, and the removed sidebar quick button must stay gone.

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

let passed = 0;
let failed = 0;
function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

console.log("\nAdd-project entry matrix");
const here = dirname(fileURLToPath(import.meta.url));
const treeSource = readFileSync(resolve(here, "../components/ProjectTree.tsx"), "utf8");
const addControlsSource = readFileSync(resolve(here, "../components/ProjectTreeAddControls.tsx"), "utf8");
const appSource = readFileSync(resolve(here, "../AppRuntime.tsx"), "utf8");
const locales = ["en", "zh"].map((name) =>
  readFileSync(resolve(here, `../locales/${name}.ts`), "utf8"),
);

// Workbench "+" menu: blank project, then an existing folder.
ok(
  /key: "blank-project"[\s\S]*?key: "open-local-folder"/.test(addControlsSource),
  "workbench + menu keeps blank project before existing folder",
);
ok(!/remote-connection/.test(addControlsSource), "no remote connection item remains in the + menu");

// Classic/creation "+": a menu rather than a direct picker.
ok(
  /project-tree__add-project[\s\S]*?aria-haspopup="menu"/.test(addControlsSource),
  "classic/creation + button opens a menu instead of the direct picker",
);
ok(
  /ProjectTreeHeaderAddControl[\s\S]*?items=\{classicHeaderAddItems\}/.test(treeSource),
  "classic/creation + menu uses the header add items",
);
ok(/project-tree__empty-primary/.test(treeSource), "empty state offers the local add-project action");

// Removed sidebar quick button: gone from App.tsx and no locale key remains.
ok(!/sidebar__new--remote/.test(appSource), "removed sidebar quick button leaves no JSX behind");
ok(!/creation\.sidebar\.remote/.test(appSource), "no creation.sidebar.remote usage in App.tsx");
for (const [index, source] of locales.entries()) {
  ok(!/creation\.sidebar\.remote/.test(source), `creation.sidebar.remote removed from locale #${index + 1}`);
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
