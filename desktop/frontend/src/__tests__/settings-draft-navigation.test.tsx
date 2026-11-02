import assert from "node:assert/strict";
import React, { act, useState } from "react";
import { JSDOM } from "jsdom";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { SettingsExperienceProvider, useSettingsDraft, useSettingsExperience } from "../components/SettingsExperience";

const dom = new JSDOM('<div id="root"></div>', { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Node: dom.window.Node, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
let setDraft: (value: string) => void;
let resolveSave: (value: boolean) => void;
let saved = 0;
let fail = false;
let defer = false;
let actionOnly = false;

function Editor() {
  const [draft, edit] = useState("");
  const [baseline, commit] = useState("");
  const [page, navigate] = useState("editor");
  setDraft = edit;
  const { requestLeave } = useSettingsExperience();
  useSettingsDraft({ label: "Draft", dirty: page === "editor" && draft !== baseline,
    canSave: draft !== "invalid", discard: () => edit(baseline),
    save: actionOnly ? undefined : async () => {
      const submitted = draft;
      const ok = defer ? await new Promise<boolean>(resolve => { resolveSave = resolve; }) : !fail;
      if (!ok) return false;
      saved++; commit(submitted); return true;
    } });
  return <><span id="page">{page}</span><span id="draft">{draft}</span><button id="leave" onClick={() => requestLeave(() => navigate("other"))}>Navigate</button></>;
}
const root = createRoot(document.getElementById("root")!);
async function mount() { await act(async () => { root.render(<LocaleProvider><SettingsExperienceProvider key={Math.random()}><Editor /></SettingsExperienceProvider></LocaleProvider>); }); }
async function click(text: string) {
  const button = [...document.querySelectorAll("button")].find(el => el.textContent?.trim() === text)!;
  assert.ok(button, text); await act(async () => { button.click(); });
}
const page = () => document.getElementById("page")?.textContent;
const dialog = () => document.querySelector(".settings-leave-dialog");

await mount();
await click("Navigate");
assert.equal(page(), "other", "clean navigation needs no confirmation");
assert.equal(dialog(), null);

await mount();
await act(async () => setDraft("first"));
await click("Navigate");
assert.ok(dialog()); assert.equal(page(), "editor");
await click("Keep editing");
assert.equal(document.getElementById("draft")?.textContent, "first");
assert.equal(dialog(), null);
await click("Navigate");
await click("Discard and leave");
assert.equal(page(), "other");
assert.equal(document.getElementById("draft")?.textContent, "");

await mount(); fail = true;
await act(async () => setDraft("retained"));
await click("Navigate"); await click("Save and continue");
assert.equal(page(), "editor"); assert.ok(dialog()?.querySelector('[role="alert"]'));
assert.equal(document.getElementById("draft")?.textContent, "retained");
fail = false;
await click("Save and continue");
assert.equal(page(), "other"); assert.equal(dialog(), null); assert.equal(saved, 1);

await mount();
await act(async () => setDraft("invalid")); await click("Navigate");
assert.equal([...document.querySelectorAll("button")].find(el => el.textContent === "Save and continue")?.disabled, true);
await click("Keep editing");
const event = new dom.window.Event("beforeunload", { cancelable: true });
window.dispatchEvent(event);
assert.equal(event.defaultPrevented, true, "reload protects dirty drafts");

await mount(); defer = true;
await act(async () => setDraft("submitted")); await click("Navigate");
const saveButton = [...document.querySelectorAll("button")].find(el => el.textContent === "Save and continue")!;
await act(async () => { saveButton.click(); });
await act(async () => setDraft("newer edit"));
await act(async () => { resolveSave(true); });
assert.equal(page(), "editor", "a newer edit cannot be silently discarded by an earlier save");
assert.ok(dialog());
defer = false; await click("Save and continue"); assert.equal(page(), "other");

actionOnly = true; await mount(); await act(async () => setDraft("unsent feedback")); await click("Navigate");
assert.equal([...document.querySelectorAll("button")].find(el => el.textContent === "Save and continue")?.disabled, true, "leaving never auto-submits action forms");
await click("Keep editing");

const saveOrder: string[] = [];
function NestedEditor({ commit }: { commit: () => void }) {
  const [dirty, setDirty] = useState(true);
  useSettingsDraft({ label: "Child", order: 100, dirty, save: async () => { saveOrder.push("child"); commit(); setDirty(false); return true; } });
  return null;
}
function ParentEditor({ initiallyDirty }: { initiallyDirty: boolean }) {
  const [dirty, setDirty] = useState(initiallyDirty);
  const [page, navigate] = useState("editor");
  const { requestLeave } = useSettingsExperience();
  useSettingsDraft({ label: "Parent", dirty, save: async () => { saveOrder.push("parent"); setDirty(false); return true; } });
  return <><span id="page">{page}</span><NestedEditor commit={() => setDirty(true)} /><button onClick={() => requestLeave(() => navigate("other"))}>Navigate</button></>;
}
for (const initiallyDirty of [false, true]) {
  saveOrder.length = 0;
  await act(async () => root.render(<LocaleProvider><SettingsExperienceProvider key={String(initiallyDirty)}><ParentEditor initiallyDirty={initiallyDirty} /></SettingsExperienceProvider></LocaleProvider>));
  await click("Navigate"); await click("Save and continue");
  assert.deepEqual(saveOrder, ["child", "parent"], "child applies before parent persistence, including an initially clean parent");
  assert.equal(page(), "other"); assert.equal(dialog(), null);
}
await act(async () => root.unmount()); dom.window.close();
console.log("PASS clean/dirty navigation, stay/discard/save, failure retention/retry, invalid and action-only drafts, reload, concurrent edits and nested parent persistence");
