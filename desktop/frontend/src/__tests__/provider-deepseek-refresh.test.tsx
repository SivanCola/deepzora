import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import type { ProviderView } from "../lib/types";

const dom = new JSDOM('<div id="root"></div>', { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window), IS_REACT_ACT_ENVIRONMENT: true,
});
const { createRoot } = await import("react-dom/client");
const { ProviderEditor } = await import("../components/SettingsPanel");
const { LocaleProvider } = await import("../lib/i18n");
const { installDesktopHostStub } = await import("./desktopHostStub");
installDesktopHostStub({ FetchProviderModelCatalog: async () => [
  { model: "deepseek-flash", displayName: "DeepSeek-V4.1-Flash", inputModalities: ["text", "image"] },
  { model: "deepseek-v4-pro", displayName: "DeepSeek-V4-Pro", inputModalities: ["text"] },
] });
const initial = {
  name: "deepseek-test", kind: "openai", baseUrl: "https://api.deepseek.com/v1",
  requestUrl: "https://api.deepseek.com/v1/chat/completions", models: ["deepseek-flash"], default: "deepseek-flash",
  apiKeyEnv: "TEST_KEY", keySet: true, added: true, builtIn: false, visionModels: [], supportedEfforts: [],
  modelsUrl: "", balanceUrl: "", contextWindow: 1000000,
} as ProviderView;
const saved: ProviderView[] = [];
const root = createRoot(document.getElementById("root")!);
await act(async () => root.render(<LocaleProvider><ProviderEditor initial={initial} kinds={["openai"]} busy={false}
  onCancel={() => {}} onSave={async (value) => { saved.push(value); }} /></LocaleProvider>));
assert.match(document.querySelector(".provider-model-draft__option")!.textContent!, /deepseek-flash/);
await act(async () => {
  (document.querySelector(".provider-model-toolbar button") as HTMLButtonElement).click();
  await new Promise(resolve => setTimeout(resolve, 20));
});
const flash = [...document.querySelectorAll(".provider-model-draft__option")]
  .find(row => row.textContent?.includes("DeepSeek-V4.1-Flash"))!;
assert.ok(flash, "refresh shows the official friendly name");
assert.equal(flash.querySelector("span[title]")?.getAttribute("title"), "deepseek-flash");
assert.equal(flash.querySelector(".provider-model-draft__id")?.textContent, "ID: deepseek-flash");
assert.equal(flash.querySelector("input")!.checked, true);
assert.equal(saved.length, 0, "refresh does not save or change the active connection");
await act(async () => {
  const save = document.querySelector<HTMLButtonElement>(".provider-editor-footer .btn--primary")!;
  assert.equal(save.disabled, false);
  save.click();
});
assert.equal(saved.length, 1);
assert.deepEqual(saved[0].models, ["deepseek-flash"]);
assert.equal(saved[0].default, "deepseek-flash");
assert.equal(saved[0].kind, "openai");
assert.equal(saved[0].baseUrl, initial.baseUrl);
assert.equal(saved[0].requestUrl, initial.requestUrl);
console.log("PASS: DeepSeek refresh changes the label only; saved model=deepseek-flash, protocol=openai, endpoint=/v1/chat/completions");
await act(async () => root.unmount());
dom.window.close();
