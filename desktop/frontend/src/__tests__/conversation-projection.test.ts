import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { WorkspaceDockRegion } from "../app-shell/WorkspaceDockRegion";
import type { Translator } from "../lib/i18n";
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
assert.equal(localView.composer.attachmentInputEnabled, true);
assert.equal(localView.composer.localDurableGuidance, true);
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
for (const chatVisible of [false, true]) for (const localToolsEnabled of [false, true]) for (const dockMode of ["files", "changed", "context"]) {
  const layout = projectConversationLayout({ chatVisible, localToolsEnabled, dockMode, dockRenderable: true,
    dockGridOpen: true, dockOverlay: true, dockOpen: true, dockMaximized: true, terminalOpen: true });
  const permitted = chatVisible && (localToolsEnabled || !["files", "changed"].includes(dockMode));
  assert.equal(layout.dockVisible, permitted);
  assert.equal(layout.dockGridOpen, permitted);
  assert.equal(layout.dockOverlay, permitted);
  assert.equal(layout.terminalOpen, chatVisible && localToolsEnabled);
  assert.equal(layout.dockMaximized, chatVisible, "automation masks stored maximization without modifying the preference");
  if (!localToolsEnabled && (dockMode === "files" || dockMode === "changed")) {
    const noop = () => {};
    const markup = renderToStaticMarkup(createElement(WorkspaceDockRegion, {
      visible: layout.dockVisible, overlay: layout.dockOverlay, mode: dockMode,
      showContext: true, t: ((key: string) => key) as Translator,
      onPickEntry: noop, context: localView.context,
      workspaceKey: "fixture", workspace: { open: layout.dockVisible, maximized: false, onClose: noop, onToggleMaximized: noop },
    }));
    assert.equal(markup, "", "actual dock region never mounts Files/Changes without local tools");
  }
}
console.log("conversation projection: shared source identity and local-tool layout policy passed");
