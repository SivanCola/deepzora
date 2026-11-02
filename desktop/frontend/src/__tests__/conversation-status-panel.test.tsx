import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { ConversationStatusPanel, type ConversationStatusPanelProps } from "../components/ConversationStatusPanel";
import { WorkspaceChangesProvider } from "../components/WorkspaceChangesProvider";
import { LocaleProvider } from "../lib/i18n";
import { resetWorkspaceRefreshStoreForTests } from "../lib/workspaceRefreshStore";
import { installDesktopHostStub } from "./desktopHostStub";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, Node: dom.window.Node,
  HTMLElement: dom.window.HTMLElement, Element: dom.window.Element, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
window.matchMedia = () => ({ matches: true, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList;
Object.assign(dom.window.HTMLElement.prototype, { attachEvent() {}, detachEvent() {} });
Object.defineProperty(dom.window.HTMLElement.prototype, "getBoundingClientRect", { value: () => ({ left: 200, top: 50, right: 500, bottom: 90, width: 300, height: 40, x: 200, y: 50, toJSON() {} }) });
const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve(); };
const waitFor = async (label: string, condition: () => boolean) => {
  for (let index = 0; index < 50; index++) {
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 15)); });
    if (condition()) return;
  }
  throw new Error(`Timed out: ${label}`);
};
const calls: string[] = [];
const stub = installDesktopHostStub({
  WorkspaceRevisionForTab: async () => ({ revisions: { content: 0, tree: 0, workingTree: 0, gitMeta: 0, session: 0 }, watchState: "active" }),
  WorkspaceChanges: async () => ({ gitAvailable: true, gitBranch: "main", added: 223, removed: 2, files: [{ path: "file.ts", sources: ["git"] }] }),
  ListSubagentRunsForTab: async (tab: string) => { calls.push(`children:${tab}`); return [{ ref: "sa_one", name: "Review changes", kind: "task", status: "running", createdAt: 1, updatedAt: 2 }]; },
  HeartbeatReloadConfig: async () => ({ tasks: [
    { id: "flow", title: "Check CI", prompt: "Inspect builds", interval: "1h", scope: "project", workspaceRoot: "/A", topicId: "topic", enabled: true, runHistory: [{ at: 10, topicId: "old-topic" }] },
    { id: "foreign", title: "Foreign workflow", scope: "project", workspaceRoot: "/elsewhere" },
  ] }),
  GitBranchesForTab: async () => ["main", "feature"],
  GitCheckoutForTab: async (tab: string, root: string, branch: string) => { calls.push(`checkout:${tab}:${root}:${branch}`); },
  RunWorkspaceGitActionForTab: async (tab: string, root: string, action: string) => { calls.push(`git:${tab}:${root}:${action}`); return "done"; },
  InspectBackgroundJobForTab: async (tab: string, id: string) => { calls.push(`output:${tab}:${id}`); return { id, kind: "bash", label: "Build", status: "running", output: "Build output" }; },
  CancelSubagentForTab: async (tab: string, ref: string) => { calls.push(`cancel-child:${tab}:${ref}`); return true; },
  ReadSubagentHistoryForTab: async (tab: string, ref: string) => {
    calls.push(`read-child:${tab}:${ref}`);
    return { status: "ready", messages: [{ messageId: "child-message", role: "assistant", inline: { content: "Real child answer" } }], hasOlder: false, hasNewer: false, snapshotSequence: 1 };
  },
});
const root = createRoot(document.getElementById("root")!);
const props: ConversationStatusPanelProps = {
  tabId: "A", scopeKey: "A", workspaceRoot: "/A", readOnly: false, running: false, goal: "Complete the rollout",
  goalView: { id: "goal", revision: 1, objective: "Complete the rollout", phase: "active", activation: "armed", maxGoalRounds: 10, roundsStarted: 2, createdAt: "", updatedAt: "" },
  todos: [{ content: "Inspect changes", status: "completed" }, { content: "Verify behavior", status: "in_progress" }],
  planText: "Approved plan details", jobs: [{ id: "bash-1", kind: "bash", label: "Build", status: "running", startedAt: 1 }], tabs: [],
  onPauseGoal: () => calls.push("pause"), onResumeGoal: () => calls.push("resume"), onContinueTodos: () => calls.push("continue"),
  onOpenChanges: () => calls.push("changes"), onOpenAutomation: () => calls.push("directory"),
  onOpenTopic: (...args) => calls.push(`topic:${args.join(":")}`),
  onCancelJob: async (tab, id) => { calls.push(`cancel-job:${tab}:${id}`); return true; },
};
const render = async () => act(async () => {
  root.render(<LocaleProvider><WorkspaceChangesProvider tabId={props.tabId} scopeKey={props.scopeKey} enabled running={false}>
    <ConversationStatusPanel key={props.scopeKey} {...props} />
  </WorkspaceChangesProvider></LocaleProvider>);
  await flush();
});
const button = (text: string) => {
  const found = [...document.querySelectorAll<HTMLButtonElement>("button")].find(element => element.textContent?.trim() === text);
  assert(found, `missing button: ${text}`); return found;
};
const click = async (text: string) => act(async () => { button(text).click(); await flush(); });
const open = async () => {
  await act(async () => (document.querySelector('[aria-label="Expand session status"]') as HTMLButtonElement).click());
  await waitFor("all sections", () => Boolean(document.querySelector('[role="dialog"]')) && document.body.textContent!.includes("Review changes"));
};
try {
  await render(); await open();
  await waitFor("Git snapshot", () => document.body.textContent!.includes("+223"));
  for (const label of ["Environment / Git", "Goal", "Plan / Todos", "Background terminals", "Workflows / Automations", "Subagents"]) assert(document.body.textContent?.includes(label), label);
  assert(!document.body.textContent?.includes("Foreign workflow"));
  await click("Pause goal"); await click("Continue");
  assert(calls.includes("pause") && calls.includes("continue"));
  await click("main"); await waitFor("branches", () => Boolean([...document.querySelectorAll("button")].find(element => element.textContent === "feature")));
  await click("feature");
  assert(calls.includes("checkout:A:/A:feature"));
  await click("Stage all"); assert(calls.includes("git:A:/A:stage"));
  await click("View output"); assert(document.body.textContent?.includes("Build output"));
  const cancelButtons = [...document.querySelectorAll<HTMLButtonElement>("button")].filter(element => element.textContent === "Cancel");
  assert.equal(cancelButtons.length, 2);
  await act(async () => { cancelButtons[0].click(); await flush(); });
  await act(async () => { cancelButtons[1].click(); await flush(); });
  assert(calls.includes("cancel-job:A:bash-1") && calls.includes("cancel-child:A:sa_one"));
  await click("Enter child conversation"); assert(document.body.textContent?.includes("Real child answer"));
  assert(calls.includes("read-child:A:sa_one"));
  await click("Back");
  await act(async () => window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape" })));
  await waitFor("collapse", () => !document.querySelector('[role="dialog"]'));
  assert.equal(document.activeElement?.getAttribute("aria-label"), "Expand session status");
  props.goalView = { ...props.goalView!, phase: "paused" };
  props.jobs = [];
  await render(); await open(); await click("Resume goal"); assert(calls.includes("resume"));
  await click("Open run conversation"); assert(calls.includes("topic:project:/A:topic"));
  await waitFor("closed after navigation", () => !document.querySelector('[role="dialog"]'));
  await open(); await click("Automation directory"); assert(calls.includes("directory"));
  props.scopeKey = "B"; props.tabId = "B"; props.workspaceRoot = "/B";
  await render();
  await waitFor("scope disposes old panel", () => !document.querySelector('[role="dialog"]'));
  props.readOnly = true;
  await render(); await open();
  await waitFor("replacement Git snapshot", () => [...document.querySelectorAll("button")].some(element => element.textContent === "Stage all"));
  assert(button("Stage all").disabled && button("Resume goal").disabled && button("Cancel").disabled,
    "read-only sessions expose inspection without mutation controls");
  console.log("PASS all status sections, scoped Git/jobs/child actions, Goal lifecycle, workflow navigation, Escape and session disposal");
} finally { await act(async () => root.unmount()); resetWorkspaceRefreshStoreForTests(); stub.uninstall(); dom.window.close(); }
