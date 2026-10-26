import assert from "node:assert/strict";
import { projectConversation, projectConversationLayout, projectNavigationSurfaceTarget } from "../app-runtime/conversationProjection";
import { initialState } from "../lib/useController";
import type { BackgroundRuntimeView } from "../lib/types";

const local = { ...initialState, running: true, activeTurnId: "local-turn", turnTokens: 999,
  turnStartAt: 111, sessionTokens: 999, sessionCost: 999, sessionCurrency: "USD",
  context: { used: 999, window: 999, sessionTokens: 999 }, balance: { available: true, display: "LOCAL" },
  meta: { ready: true, eventChannel: "local-events", cwd: "local-cwd", label: "local-model", workspaceName: "local-project", gitBranch: "local-branch",
    imageInputEnabled: true, visionFallbackEnabled: true, pinnedFiles: [{ path: "local-file", sizeBytes: 10, tokenEstimate: 5 }] },
} as typeof initialState;
const background = [{ id: "local-runtime" }] as unknown as BackgroundRuntimeView[];
const localView = projectConversation({ local, activeTabId: "local", backgroundRuntimes: background, connectingLabel: "connecting" });
assert.equal(localView.runtime, local);
assert.equal(localView.status.backgroundRuntimes, background);
assert.equal(localView.context.tabId, "local");
const readableBeforeRuntime = projectNavigationSurfaceTarget({
  activeTabId: "local",
  sessionKey: "local:session",
  local: {
    ...local,
    meta: { ...local.meta!, ready: false, runtime: { phase: "starting", epoch: "runtime-1" } },
    backendActivationPending: true,
    hydrating: false,
    hydrateError: undefined,
  },
});
assert.equal(readableBeforeRuntime.ready, true, "local navigation paint is released by readable history rather than runtime readiness");
assert.equal(readableBeforeRuntime.backendActivationPending, false, "local runtime activation does not block transcript paint");
const localHistoryPending = projectNavigationSurfaceTarget({
  activeTabId: "local",
  sessionKey: "local:session",
  local: { ...local, backendActivationPending: true, hydrating: true, hydrateError: undefined },
});
assert.equal(localHistoryPending.ready, false, "local cache miss keeps only the target history skeleton pending");
for (const chatVisible of [false, true]) {
  const layout = projectConversationLayout({ chatVisible, dockRenderable: true,
    dockGridOpen: true, dockOverlay: true, dockOpen: true, dockMaximized: true, terminalOpen: true });
  assert.equal(layout.dockVisible, chatVisible);
  assert.equal(layout.dockGridOpen, chatVisible);
  assert.equal(layout.dockOverlay, chatVisible);
  assert.equal(layout.terminalOpen, chatVisible);
  assert.equal(layout.dockMaximized, chatVisible, "automation masks stored maximization without modifying the preference");
}
console.log("conversation projection: shared source identity and layout policy passed");
