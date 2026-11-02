import assert from "node:assert/strict";
import React, { act, StrictMode, useRef } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { useBranchSwitcher } from "../lib/useBranchSwitcher";
import { installDesktopHostStub } from "./desktopHostStub";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window, document: dom.window.document, Node: dom.window.Node,
  localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true,
});
const lists: { tab: string; root: string; request: ReturnType<typeof deferred<string[]>> }[] = [];
const switches: { tab: string; root: string; branch: string; request: ReturnType<typeof deferred<void>> }[] = [];
const stub = installDesktopHostStub({
  GitBranchesForTab(tab: string, root: string) {
    const request = deferred<string[]>(); lists.push({ tab, root, request }); return request.promise;
  },
  GitCheckoutForTab(tab: string, root: string, branch: string) {
    const request = deferred<void>(); switches.push({ tab, root, branch, request }); return request.promise;
  },
  GitCreateBranchForTab(tab: string, root: string, branch: string) {
    const request = deferred<void>(); switches.push({ tab, root, branch, request }); return request.promise;
  },
});
let branch!: ReturnType<typeof useBranchSwitcher>;
let changes = 0;
function Harness({ scope }: { scope: string }) {
  const rootRef = useRef<HTMLDivElement>(null);
  branch = useBranchSwitcher({
    tabId: "reused-tab", scopeKey: scope, workspaceRoot: "/" + scope, gitBranch: "main", rootRef,
    onBranchChanged: () => { changes++; },
  });
  return <div ref={rootRef} />;
}
const root = createRoot(document.getElementById("root")!);
const render = async (scope: string) => act(async () => {
  root.render(<StrictMode><Harness scope={scope} /></StrictMode>);
  await flush();
});
await render("A");
await act(async () => { branch.toggleBranchMenu(); branch.setBranchQuery("old"); await flush(); });
assert.equal(lists[0].root, "/A");
await act(async () => { void branch.checkoutBranch("shared"); await flush(); });
assert.equal(switches[0].root, "/A");
await render("B");
assert.equal(branch.branchQuery, "");
assert.deepEqual(branch.branches, []);
assert.equal(branch.branchSwitchErr, "");
await act(async () => { branch.toggleBranchMenu(); await flush(); });
assert.equal(lists[1].root, "/B", "same tab and same branch still carry project identity");
await act(async () => {
  lists[1].request.resolve(["main", "shared", "B-only"]);
  lists[0].request.resolve(["A-only"]);
  switches[0].request.resolve();
  await flush();
});
assert.deepEqual(branch.branches, ["main", "shared", "B-only"]);
assert.equal(branch.activeBranch, "main", "late A checkout cannot change B's branch");
assert.equal(changes, 0, "late A checkout cannot refresh B");
await act(async () => { void branch.checkoutBranch("shared"); void branch.checkoutBranch("shared"); await flush(); });
assert.equal(switches.length, 2, "duplicate checkout clicks coalesce synchronously");
await act(async () => { switches[1].request.resolve(); await flush(); });
assert.equal(branch.activeBranch, "shared");
assert.equal(changes, 1);
assert.equal(lists.length, 3, "successful checkout refreshes its own branch list");
await act(async () => { void branch.createBranch("new"); await flush(); });
await render("C");
await act(async () => { switches[2].request.reject(new Error("A stale failure")); await flush(); });
assert.equal(branch.branchSwitchErr, "");
await act(async () => root.unmount());
stub.uninstall(); dom.window.close();
console.log("PASS scoped branch races and stale checkout/create");
