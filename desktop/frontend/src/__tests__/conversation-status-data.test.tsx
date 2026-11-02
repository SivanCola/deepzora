import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { useConversationStatusData } from "../lib/useConversationStatusData";
import { installDesktopHostStub } from "./desktopHostStub";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true });
let visibility = "visible";
Object.defineProperty(document, "visibilityState", { get: () => visibility });
const root = createRoot(document.getElementById("root")!);
const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve(); };
const waitFor = async (label: string, condition: () => boolean) => {
  for (let i = 0; i < 100; i++) {
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 10)); });
    if (condition()) return;
  }
  throw new Error(`Timed out: ${label}`);
};
let topic = "previous-run";
let reads = 0;
const tasks = () => [
  { id: "a", title: "Workflow A", scope: "project", workspaceRoot: "/A", topicId: topic, runHistory: [{ at: 1, topicId: topic }] },
  { id: "b", title: "Workflow B", scope: "project", workspaceRoot: "/B", topicId: "run-b" },
];
const stub = installDesktopHostStub({
  ListSubagentRunsForTab: async (tab: string) => [{ ref: `child-${tab}`, name: tab }],
  HeartbeatReloadConfig: async () => { reads++; return { tasks: tasks() }; },
});
let tabId = "A";
let open = true;
let data: ReturnType<typeof useConversationStatusData>;
function Harness() {
  data = useConversationStatusData(tabId, tabId, `/${tabId}`, open);
  return <div>{JSON.stringify({ workflows: data.workflows, subagents: data.subagents })}</div>;
}
const render = async () => act(async () => { root.render(<Harness />); await flush(); });
const emit = async (kind: string, tab = "automation-tab") => act(async () => {
  stub.emit("agent:event", { kind, tabId: tab });
  await flush();
});
try {
  await render();
  await waitFor("initial workflow", () => data.workflows[0]?.topicId === "previous-run");
  const initialReads = reads;
  topic = "new-background-run";
  // Automation starts in a different tab; the collapsed task record and its
  // history must follow the new run without a manual reload.
  await emit("turn_started");
  await emit("notice");
  await emit("turn_done");
  await waitFor("cross-tab workflow history", () => data.workflows[0]?.runHistory?.[0]?.topicId === topic);
  assert.equal(reads, initialReads + 1, "lifecycle bursts should coalesce");
  assert.equal(data.workflows.length, 1, "foreign project workflow leaked");
  assert.equal(data.subagents[0]?.ref, "child-A");

  // Streaming in unrelated chats must not rescan workflow configuration.
  const settledReads = reads;
  await emit("text"); await emit("tool_result");
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)); });
  assert.equal(reads, settledReads);

  // Closing/hidden pages do not start background reads; visibility restores
  // the authoritative run history.
  open = false; await render();
  await emit("turn_done");
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)); });
  assert.equal(reads, settledReads);
  open = true; await render();
  await waitFor("reopen", () => reads > settledReads);
  const visibleReads = reads;
  visibility = "hidden";
  topic = "completed-while-hidden";
  await emit("turn_done");
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)); });
  assert.equal(reads, visibleReads);
  visibility = "visible";
  await act(async () => { document.dispatchEvent(new dom.window.Event("visibilitychange")); });
  await waitFor("visibility refresh", () => data.workflows[0]?.topicId === topic);

  // A late response from an old session cannot repopulate a replacement
  // session, including a request triggered by another automation tab.
  let release!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  let holding = false;
  stub.commands.HeartbeatReloadConfig = async () => {
    reads++; holding = true;
    const snapshot = tasks();
    await held;
    return { tasks: snapshot };
  };
  await emit("turn_done");
  await waitFor("old request started", () => holding);
  tabId = "B";
  await render();
  assert.equal(data.workflows.length, 0);
  assert.equal(data.subagents.length, 0);
  stub.commands.HeartbeatReloadConfig = async () => { reads++; return { tasks: tasks() }; };
  await act(async () => { release(); await flush(); });
  await waitFor("replacement scope", () => data.workflows[0]?.topicId === "run-b");
  assert.equal(data.workflows.length, 1);
  assert.equal(data.subagents[0]?.ref, "child-B");
  assert(!document.body.textContent?.includes("Workflow A"));
  console.log("PASS cross-tab workflow lifecycle/history, coalescing, visibility, closed panel and stale session responses");
} finally {
  await act(async () => root.unmount());
  stub.uninstall(); dom.window.close();
}
