import type { State } from "../lib/useController";
import type { BackgroundRuntimeView, TabMeta } from "../lib/types";

type Input = {
  local: State;
  tab?: Pick<TabMeta, "id" | "label" | "workspaceName">;
  activeTabId?: string;
  backgroundRuntimes: BackgroundRuntimeView[];
  connectingLabel: string;
};

/** Readiness and paint identity must come from the surface that will render. */
export function projectNavigationSurfaceTarget(input: {
  activeTabId?: string; sessionKey: string;
  local: Pick<State, "meta" | "backendActivationPending" | "hydrating" | "hydrateError">;
}) {
  const { local } = input;
  // Local history has its own controller-independent canonical read path.
  // Navigation paint therefore waits for the readable history cut, not for
  // provider/MCP/lease runtime readiness. Write actions remain fenced by
  // controllerReady in the composer and command owners.
  return {
    activeTabId: input.activeTabId,
    sessionKey: input.sessionKey,
    ready: Boolean(input.activeTabId) && !local.hydrating && !local.hydrateError,
    backendActivationPending: false,
    hydrating: Boolean(local.hydrating),
    hydrateError: local.hydrateError,
  };
}

/** Display-only projection of the active local conversation. */
export function projectConversation({ local, activeTabId, backgroundRuntimes, connectingLabel }: Input) {
  const runtime = local;
  const modelLabel = local.meta?.label;
  const timing = {
    turnPhase: runtime.turnPhase, turnStartAt: runtime.turnStartAt,
    turnDoneAt: runtime.turnDoneAt, lastTurnOutputTokens: runtime.lastTurnOutputTokens,
    lastTurnWaitAccumMs: runtime.lastTurnWaitAccumMs,
    turnWaitAccumMs: runtime.turnWaitAccumMs, promptWaitStartedAt: runtime.promptWaitStartedAt,
    turnTokens: runtime.turnTokens, turnOutputTokens: runtime.turnOutputTokens,
    turnOutputCharsAtUsage: runtime.turnOutputCharsAtUsage,
    turnModelActiveAt: runtime.turnModelActiveAt, turnModelActiveMs: runtime.turnModelActiveMs,
    turnRateOutputQuarters: runtime.turnRateSample?.outputQuarters,
    turnArgChars: runtime.turnArgChars, retry: runtime.retry,
    turnOutputEstimated: runtime.turnOutputEstimated,
    lastTurnOutputEstimated: runtime.lastTurnOutputEstimated,
    readStatuses: runtime.readStatuses,
  };
  return {
    runtime,
    localToolsEnabled: true,
    composer: {
      ...timing,
      running: local.running,
      goalStatus: local.meta?.goalStatus,
      goalView: local.meta?.goalView,
      goalRuntime: local.meta?.goalRuntime,
      cwd: local.meta?.cwd,
      modelLabel: modelLabel || connectingLabel,
      imageInputEnabled: local.meta?.imageInputEnabled !== false,
      imageUnderstandingEnabled: local.meta?.visionFallbackEnabled === true,
      attachmentInputEnabled: true,
      pinnedFiles: local.meta?.pinnedFiles,
      turnId: local.activeTurnId,
      effort: local.effort,
      localDurableGuidance: true,
      context: runtime.context, turnCost: runtime.turnCost, turnRateBand: runtime.turnRateBand,
      currency: runtime.sessionCurrency, cacheHitTokens: runtime.usage?.cacheHitTokens,
      cacheMissTokens: runtime.usage?.cacheMissTokens, balance: runtime.balance,
    },
    context: {
      tabId: activeTabId,
      items: runtime.items, context: runtime.context, usage: runtime.usage,
      sessionTokens: runtime.sessionTokens, sessionCost: runtime.sessionCost,
      sessionCurrency: runtime.sessionCurrency, turnTokens: runtime.turnTotalTokens,
      turnCost: runtime.turnCost, turnRateBand: runtime.turnRateBand, balance: runtime.balance,
      sessionGen: runtime.sessionGen, usageSeq: runtime.usageSeq,
    },
    status: {
      context: runtime.context, usage: runtime.usage, balance: runtime.balance,
      running: runtime.running, jobs: runtime.jobs,
      backgroundRuntimes,
      sessionTokens: runtime.sessionTokens, turnTokens: runtime.turnTotalTokens,
      lastTurnOutputTokens: runtime.lastTurnOutputTokens, lastTurnModelMs: runtime.lastTurnModelMs,
      lastTurnOutputEstimated: runtime.lastTurnOutputEstimated, lastRequestTps: runtime.lastRequestTps,
      turnCost: runtime.turnCost, turnRateBand: runtime.turnRateBand, cost: runtime.sessionCost,
      currency: runtime.sessionCurrency, modelLabel,
      workspacePath: local.meta?.workspacePath || local.meta?.workspaceRoot || local.meta?.cwd,
      workspaceName: local.meta?.workspaceName,
      gitBranch: local.meta?.gitBranch,
    },
  };
}

export function projectConversationLayout(input: {
  chatVisible: boolean; localToolsEnabled: boolean; dockMode: string;
  dockRenderable: boolean; dockGridOpen: boolean; dockOverlay: boolean;
  dockOpen: boolean; dockMaximized: boolean; terminalOpen: boolean;
}) {
  const localDockBlocked = !input.localToolsEnabled && (input.dockMode === "files" || input.dockMode === "changed");
  const dockVisible = input.chatVisible && input.dockRenderable && !localDockBlocked;
  return {
    dockVisible,
    dockGridOpen: input.chatVisible && input.dockGridOpen && !localDockBlocked,
    dockOverlay: dockVisible && input.dockOverlay,
    dockMaximized: input.chatVisible && input.dockOpen && input.dockMaximized,
    terminalOpen: input.chatVisible && input.terminalOpen && input.localToolsEnabled,
  };
}

/** Workspace controller scope key: any identity input change re-scopes the composer. */
export function projectWorkspaceScopeKey(input: {
  activeTabId: string | undefined;
  sessionKey: string;
  cwd: string | undefined;
  sessionGen: number;
  workspaceControllerEpoch: number;
}): string {
  return [
    input.activeTabId ?? "",
    input.sessionKey,
    input.cwd ?? "",
    input.sessionGen,
    input.workspaceControllerEpoch,
  ].join("\u0000");
}

// Workspace navigation belongs to the project, not to a single conversation.
// A session switch inside the same project must therefore retain the dock,
// tree and selection state.
export function projectWorkspaceTreeMemoryKey(input: {
  scope: string | undefined;
  workspaceRoot: string | undefined;
  cwd: string | undefined;
}): string {
  return [
    input.scope ?? "",
    input.workspaceRoot ?? input.cwd ?? "",
  ].join("\u0000");
}
