import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { WorkspaceChangesProvider } from "../components/WorkspaceChangesProvider";
import { WorkspaceChangesBadge } from "../components/WorkspaceChangesBadge";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import { useWorkspaceRefresh, resetWorkspaceRefreshStoreForTests } from "../lib/workspaceRefreshStore";
import { LocaleProvider } from "../lib/i18n";
import type { WorkspaceChangesView } from "../lib/types";
import { installDesktopHostStub } from "./desktopHostStub";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, Node: dom.window.Node,
  localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
let visible = true;
Object.defineProperty(document, "visibilityState", { get: () => visible ? "visible" : "hidden" });
const timers = new Map<number, () => void>();
let timerId = 0;
window.setTimeout = ((callback: () => void) => { timers.set(++timerId, callback); return timerId; }) as typeof window.setTimeout;
window.clearTimeout = id => { timers.delete(id); };
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const tick = async () => act(async () => {
  const pending = [...timers.values()]; timers.clear();
  pending.forEach(callback => callback());
  await flush();
});
const requests: { tab: string; resolve: (data: WorkspaceChangesView) => void; reject: (error: Error) => void }[] = [];
const stub = installDesktopHostStub({
  WorkspaceRevisionForTab: async () => ({ revisions: { content: 0, tree: 0, workingTree: 0, gitMeta: 0, session: 0 }, watchState: "active" }),
  WorkspaceChanges: (tab: string) => new Promise<WorkspaceChangesView>((resolve, reject) => { requests.push({ tab, resolve, reject }); }),
});
const root = createRoot(document.getElementById("root")!);
let shared!: ReturnType<typeof useWorkspaceChangesResource>;
let opens = 0;
function Dock({ scope, enabled }: { scope: string; enabled: boolean }) {
  shared = useWorkspaceChangesResource("tab", scope, 0);
  useWorkspaceRefresh("tab", scope, enabled);
  return <output>{shared.workspaceChanges?.added}</output>;
}
const render = async (scope = "A", dock = true) => act(async () => {
  root.render(<WorkspaceChangesProvider tabId="tab" scopeKey={scope} enabled running={false}>
    <LocaleProvider>
      <WorkspaceChangesBadge tabId="tab" scopeKey={scope} onOpenChanges={() => { opens++; }} />
      <Dock scope={scope} enabled={dock} />
    </LocaleProvider>
  </WorkspaceChangesProvider>);
  await flush();
});
const changes = (added: number, incomplete = false): WorkspaceChangesView => ({
  files: [{ path: "file.ts", sources: ["git"], gitStatus: "M" }],
  gitAvailable: true, added, removed: 2, incomplete,
});
const emit = async (revision: number) => act(async () => {
  stub.emit("agent:event", { kind: "workspace_changed", tabId: "tab", workspace: {
    revisions: { content: revision, tree: revision, workingTree: revision, gitMeta: 0, session: 0 },
    changes: [], allPaths: true, source: "filesystem", watchState: "active",
  } });
  await flush();
});

try {
  await render();
  await tick();
  assert.equal(requests.length, 1);
  await act(async () => { void shared.loadWorkspaceChanges(); void shared.loadWorkspaceChanges(); await flush(); });
  assert.equal(requests.length, 1, "summary and dock coalesce concurrent reads");
  await act(async () => { requests[0].resolve(changes(223)); await flush(); });
  assert.equal(document.querySelector("output")?.textContent, "223");
  assert(document.querySelector(".workspace-changes-badge")?.textContent?.includes("+223"));
  await act(async () => (document.querySelector(".workspace-changes-badge") as HTMLButtonElement).click());
  assert.equal(opens, 1);

  await render("A", false);
  await emit(1); await emit(2); await emit(3); await tick();
  assert.equal(requests.length, 2, "closing the dock keeps summary events active; bursts coalesce");
  await act(async () => { requests[1].reject(new Error("temporary failure")); await flush(); });
  assert(document.querySelector(".workspace-changes-badge")?.textContent?.includes("~"), "failed refresh marks retained counts as estimates");
  assert.equal(document.querySelector("output")?.textContent, "223");

  await act(async () => { visible = false; document.dispatchEvent(new dom.window.Event("visibilitychange")); await flush(); });
  await emit(4); await tick();
  assert.equal(requests.length, 2, "hidden pages do not scan Git");
  await act(async () => { visible = true; document.dispatchEvent(new dom.window.Event("visibilitychange")); await flush(); });
  await tick();
  assert.equal(requests.length, 3, "foreground return reconciles changes");
  await render("B", false); await tick();
  assert.equal(document.querySelector(".workspace-changes-badge"), null, "a new scope never paints A's counts");
  assert.equal(requests.length, 3, "the summary scheduler coalesces replacement reads until the running read settles");
  await act(async () => { requests[2].resolve(changes(999)); await flush(); });
  assert.equal(requests.length, 4);
  assert.equal(document.querySelector(".workspace-changes-badge"), null, "late A completion cannot leak into B");
  await act(async () => { requests[3].resolve({ files: [], gitAvailable: true }); await flush(); });
  assert.equal(document.querySelector(".workspace-changes-badge"), null, "clean workspaces hide the badge");
  await act(async () => { void shared.loadWorkspaceChanges(); await flush(); });
  await act(async () => { requests[4].resolve(changes(0, true)); await flush(); });
  assert(document.querySelector(".workspace-changes-badge")?.textContent?.includes("~"), "incomplete counts never appear exact");
  console.log("PASS workspace summary sharing, watcher lifetime, coalescing, visibility, failures and scope races");
} finally {
  await act(async () => root.unmount());
  resetWorkspaceRefreshStoreForTests(); stub.uninstall(); dom.window.close();
}
