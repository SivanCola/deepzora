import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { LocaleProvider } from "../lib/i18n";
import { installDesktopHostStub } from "./desktopHostStub";
import { MCPServersSettingsPage, PluginsSettingsPage, SkillsSettingsPage } from "../components/CapabilitiesPanel";
import { StorageSettingsPage } from "../components/StorageSettingsPage";
import { MemorySettingsPage } from "../components/MemoryPanel";
import { SettingsPanel } from "../components/SettingsPanel";
import { SettingsExperienceProvider } from "../components/SettingsExperience";
import { baseSettings, flushPromises, installCanvasMock } from "../test-support/settingsTestFixtures";

const dom = new JSDOM('<div id="root"></div>', { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Node: dom.window.Node, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
const el = document.getElementById("root")!;
const root = createRoot(el);
installCanvasMock(dom.window as unknown as Window);
const host = installDesktopHostStub({});
function HooksPage() { return <SettingsPanel initialTab="hooks" desktopPlatform="linux" onClose={() => {}} onChanged={() => {}} onUseSubagent={() => {}} />; }
const cases = [
  ["MCPServers", MCPServersSettingsPage, []],
  ["Plugins", PluginsSettingsPage, []],
  ["SkillsSettings", SkillsSettingsPage, { skills: [], tools: [], problems: [] }],
  ["StorageSettings", StorageSettingsPage, { defaultWorkspace: "/work", statePath: "/state", cachePath: "/cache", extensionsPath: "/extensions" }],
  ["HooksSettings", HooksPage, { scope: "global", path: "/hooks", hooks: [], events: [] }],
  ["Memory", MemorySettingsPage, { available: false }],
] as const;
try {
  for (const [method, Component, result] of cases) {
    let rejectRead!: (error: Error) => void;
    const pending = new Promise((_resolve, reject) => { rejectRead = reject; });
    const commands: Record<string, unknown> = {
      Meta: async () => ({ workspaceRoot: `/test/${method}`, eventChannel: method }),
      ListTabs: async () => [],
      Settings: async () => baseSettings(),
      [method]: () => pending,
    };
    host.replaceCommands(commands);
    await act(async () => { root.render(<LocaleProvider><SettingsExperienceProvider key={method}><Component /></SettingsExperienceProvider></LocaleProvider>); await flushPromises(); });
    assert.ok(el.querySelector('[role="status"][aria-busy="true"]'), `${method}: pending read exposes loading`);
    await act(async () => { rejectRead(new Error(`${method} read failed`)); await flushPromises(); });
    assert.ok(el.querySelector('[role="alert"]'), `${method}: rejection exposes error`);
    assert.equal(el.querySelector('[role="status"][aria-busy="true"]'), null, `${method}: error stops loading`);
    assert.equal(el.querySelector(".mem-empty"), null, `${method}: error is not an empty result`);
    host.commands[method] = async () => result;
    const retry = [...el.querySelectorAll("button")].find(button => button.textContent === "Retry");
    assert.ok(retry, `${method}: retry is reachable even after initial failure`);
    await act(async () => { retry.click(); await flushPromises(); });
    assert.equal(el.querySelector('[role="alert"]'), null, `${method}: successful retry clears error`);
    assert.equal(el.querySelector('[role="status"][aria-busy="true"]'), null, `${method}: successful retry settles`);
    console.log(`PASS ${method}: pending, failed, retry and settled states`);
  }
} finally {
  await act(async () => root.unmount()); host.uninstall(); dom.window.close();
}
