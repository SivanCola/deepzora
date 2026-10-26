import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import { projectSessionAvailability } from "../lib/sessionAvailability";
import { activeLeaseBlockedTab } from "../lib/tabMetaRefresh";
import { topicTitle } from "../lib/sessionTitles";
import { composerDraftKeyForTab } from "../lib/composerDraftKey";
import { useWindowStatePersistence, useViewportHeightVar } from "../lib/windowState";
import { useManagementWorkspace } from "../lib/useManagementWorkspace";
import { reportPendingRevisionFailure, usePendingPlanRevisions } from "../lib/usePendingPlanRevisions";
import { useComposerModeActions } from "../lib/useComposerModeActions";
import type { CollaborationMode, TabMeta } from "../lib/types";
import type { ComposerProfile, UserPlanModeIntents } from "../lib/composerProfile";
import type { State } from "../lib/useController";
import type { Translator } from "../lib/i18n";
import type { useAppRuntimeAdapter } from "./useAppRuntimeAdapter";
import type { useNavigationSurface } from "../lib/useNavigationSurface";
import { desktopBridge } from "./desktopBridgeAdapter";
import { useSessionOperations } from "./useSessionOperations";
import { useComposerInsertCommands } from "./useComposerInsertCommands";
import { useSessionClearCommands } from "./useSessionClearCommands";
import { useRuntimeStatus } from "./useRuntimeStatus";
import { useAppDiagnostics, useRecoverableErrorToasts } from "./useAppEffectHosts";
import { useActiveTabUiReset, useDecisionSurfaceFocus } from "./useLocalUiLifecycles";
import { useActiveTabMirrorCommit } from "./activeTabMirror";
import { useInvocationMetadata } from "./useInvocationMetadata";
import { useFooterHeightLifecycle } from "./useFooterHeightLifecycle";
import { useNativeSettingsEvent } from "./useNativeSettingsEvent";
import { useWindowsMaximisedSync } from "./useNativeWindowController";
import { useShellGeometry } from "./useShellGeometry";
import { useSessionSummary } from "./useSessionSummary";
import { useComposerProfileProjection } from "./useComposerProfileProjection";
import { useTabBarCommands } from "./useTabBarCommands";
import { useExtensionSurface } from "./useExtensionSurface";
import { useTabProjectionLifecycle } from "./useTabProjectionLifecycle";
import { useSessionUndo } from "./useSessionUndo";
import { useSessionSubmission } from "../lib/useSessionSubmission";
import { useControllerProfileCommands } from "../lib/useControllerProfileCommands";
import { noteNavigationComposerEnabled } from "../lib/sessionDiagnostics";
import { useSessionPromptCommands } from "./useSessionPromptCommands";
import { useSessionControlCommands } from "./useSessionControlCommands";
import { useTodoPanelCommands } from "./useTodoPanelCommands";
import { useSessionExportCommands } from "./useSessionExportCommands";
import { useComposerRouter } from "./useComposerRouter";
import { useComposerGoalCommands } from "./useComposerGoalCommands";
import { useRuntimeEventHandlers } from "./useRuntimeEventHandlers";
import { probeProviderSetupState } from "./StartupGateLifecycle";
import { useSessionBannerCommands } from "./useSessionBannerCommands";
import { useWorkspacePanelCommands } from "./useWorkspacePanelCommands";
import { useTurnVerificationCommands } from "./useTurnVerificationCommands";
import { useTerminalPanelCommands } from "./useTerminalPanelCommands";
import { useAutomationNavigation } from "./useAutomationNavigation";
import { useDesktopNavigation } from "./useDesktopNavigation";
import { useTranscriptSurfaceProjection } from "./useTranscriptSurfaceProjection";
import { projectConversation, projectConversationLayout, projectWorkspaceScopeKey, projectWorkspaceTreeMemoryKey } from "./conversationProjection";
import { projectControllerProfiles, projectVisibleTabs } from "./controllerProfileOwner";
import { projectDecisionSurface, type AppDecisionSurfaceKind } from "./decisionSurfaceProjection";
import { createSubmissionPorts, projectSubmissionResources } from "./desktopSubmissionAdapter";
import type { useAppShellStores } from "./useAppShellStores";

const WORKSPACE_RESIZER_WIDTH = 8;

type Runtime = ReturnType<typeof useAppRuntimeAdapter>;
type Shell = ReturnType<typeof useAppShellStores>;
type Surface = ReturnType<typeof useNavigationSurface>;
type LiveStore = Runtime["snapshot"]["liveStore"];

export type AppSessionCompositionInput = {
  runtime: Runtime;
  t: Translator;
  showToast: (message: string, level?: "info" | "warn" | "error", options?: { durationMs?: number }) => void;
  shell: Shell;
  core: {
    state: State;
    liveStore: LiveStore;
    activeTabId: string | undefined;
    notice: Runtime["snapshot"]["notice"];
    activeTab: TabMeta | undefined;
    activeSessionIdentity: string;
    sessionSurfaceFence: ReturnType<typeof import("./sessionTarget").createSessionSurfaceFence>;
    sessionOperations: ReturnType<typeof useSessionOperations>;
  };
  surface: Surface;
  stores: {
    composerProfilesByTab: Record<string, ComposerProfile>;
    setComposerProfilesByTab: React.Dispatch<React.SetStateAction<Record<string, ComposerProfile>>>;
    tabMetas: TabMeta[];
    setTabMetas: React.Dispatch<React.SetStateAction<TabMeta[]>>;
    tabOrderIds: string[];
    setTabOrderIds: React.Dispatch<React.SetStateAction<string[]>>;
    userPlanModeByTabRef: { current: UserPlanModeIntents };
  };
  local: {
    setHistView: React.Dispatch<React.SetStateAction<import("./historyViewProjection").HistoryViewState | null>>;
    setTabRevealSignal: React.Dispatch<React.SetStateAction<number>>;
    workspaceScopeActiveTabRef: { current: string | undefined };
    workspaceControllerEpoch: number;
    setWorkspaceControllerEpoch: React.Dispatch<React.SetStateAction<number>>;
    dockRefreshKey: number;
    setDockRefreshKey: React.Dispatch<React.SetStateAction<number>>;
    fileRefRefreshKey: number;
    setFileRefRefreshKey: React.Dispatch<React.SetStateAction<number>>;
    projectRevision: number;
    setProjectRevision: React.Dispatch<React.SetStateAction<number>>;
  };
  goal: {
    runGoalAction: ReturnType<typeof import("../lib/goalAction").useGoalActionHandler>["runGoalAction"];
    handleGoalActionError: ReturnType<typeof import("../lib/goalAction").useGoalActionHandler>["handleGoalActionError"];
  };
};

/**
 * Session/composer composition: runs every session-domain owner hook in the
 * App body's original order and returns the bags the navigation composition
 * and the shell view consume. Pure relocation — hook order within the
 * segment is unchanged.
 */
export function useAppSessionComposition(input: AppSessionCompositionInput) {
  const { t, showToast, shell, runtime } = input;
  useRecoverableErrorToasts(showToast);
  const {
    state, liveStore, activeTabId, notice, activeTab, activeSessionIdentity, sessionOperations,
  } = input.core;
  const {
    transitioning: runtimeTransitioning, commitRendered: commitRenderedTranscriptSurface,
    begin: beginNavigationSurface, maskTarget: settleNavigationSurface, commitPaint: commitNavigationSurfacePaint,
  } = input.surface;
  const {
    sendToTab, runShellForTab, cancel, cancelForTab,
    setControllerModeForTab, setCollaborationMode: setControllerCollaborationMode,
    setCollaborationModeForTab: setControllerCollaborationModeForTab,
    setToolApprovalModeForTab,
    setComposerProfileForTab: setControllerComposerProfileForTab, setGoalForTab: setControllerGoalForTab,
    editGoalForTab: editControllerGoalForTab,
    resumeGoalForTab: resumeControllerGoalForTab, pauseGoalForTab: pauseControllerGoalForTab,
    clearGoalForTab: clearControllerGoalForTab,
    setModelForTab, setEffortForTab,
  } = runtime.composer;
  const {
    approveForTab, isPromptCurrentForTab, resolvePlanDecisionForTab,
    answerQuestionForTab, answerMCPInteractionForTab, dismissExtensionForm, drainExtensionNotifications,
    clearSession, newSession, loadOlderHistory, loadNewerHistory, navigateToTurn, rewindForTab, rewindForTabDetailed, undoRewindForTab, forkTurnForTab,
    listSessions, openChannelSession, resumeSession,
  } = runtime.sessionActions;
  const {
    switchTab, closeTab, reorderTabs, createIsolatedWorktree,
    noteNavigationIntent, registeredNavigationIntent, isNavigationIntentCurrent, reassertVisibleTabAfterStaleNavigation,
    commitSingleSurfaceNavigation, activateTopic,
    ensureBlankSurface, openCanonicalSession,
  } = runtime.navigation;
  const {
    setTransientOverlayDismissSignal, managementActive,
    windowsFramelessChrome,
    workspacePanelOpen, workspacePanelMaximized, liveTerminalHeight, setLiveWorkspacePanelRenderWidth,
    setRightDockTreeWidth, terminalPanelOpen, setSettingsTarget, enterConversation,
  } = shell;
  const { reloadConfigWarnings } = shell.preferences;
  const {
    composerProfilesByTab, setComposerProfilesByTab, tabMetas, setTabMetas, tabOrderIds, setTabOrderIds,
    userPlanModeByTabRef,
  } = input.stores;
  const {
    setHistView, setTabRevealSignal,
    workspaceScopeActiveTabRef, workspaceControllerEpoch, setWorkspaceControllerEpoch,
    setDockRefreshKey, projectRevision, setProjectRevision,
  } = input.local;
  const { runGoalAction, handleGoalActionError } = input.goal;
  const insertCommands = useComposerInsertCommands({
    activeTabId,
    sessionKey: activeSessionIdentity,
    browserSessionPath: activeTab?.sessionPath ?? state.meta?.sessionPath,
    approval: state.approval,
    operations: sessionOperations,
    t,
    showToast,
    ports: { terminalOutput: (tabId, terminalSessionId) => desktopBridge.terminalOutputForTab(tabId, terminalSessionId) },
  });
  const {
    setInsertTarget: setWorkspaceInsertTarget, replaceComposerInsert,
  } = insertCommands;
  useWindowsMaximisedSync(windowsFramelessChrome);
  const clearCommands = useSessionClearCommands({
    activeTabId,
    activeSessionIdentity,
    t,
    notice,
    operations: sessionOperations,
    refreshDock: () => setDockRefreshKey((value) => value + 1),
    ports: {
      clearSession,
    },
  });
  const { clearContextPending, setClearContextPending } = clearCommands;
  const appRef = useRef<HTMLDivElement>(null);
  const layoutRef = useRef<HTMLDivElement>(null);
  useManagementWorkspace(layoutRef, managementActive);

  // Persist window geometry across launches.
  useWindowStatePersistence();
  useViewportHeightVar();

  const { backgroundRuntimes, workspaceConflict, setWorkspaceConflict, refreshBackgroundRuntimes } = useRuntimeStatus({
    tabId: activeTabId, sessionKey: activeSessionIdentity, running: state.running,
  });

  const closeTransientOverlays = useCommittedCommand(() => {
    setTransientOverlayDismissSignal((signal) => signal + 1);
  });


  useNativeSettingsEvent({ closeTransientOverlays, setSettingsTarget });

  const [footerHeight, setFooterHeight] = useState(0);
  const footerRef = useRef<HTMLElement>(null);
  const commitFooterHeight = useCommittedCommand((height: number) => setFooterHeight(height));
  useFooterHeightLifecycle(footerRef, commitFooterHeight);
  useActiveTabMirrorCommit(activeTabId);
  const { invocationMetadataByTab, handleInvocationMetadataChange } = useInvocationMetadata();
  const shellGeometry = useShellGeometry({ appRef, layoutRef });
  const {
    chatReservedWidth,
    workspacePanelRenderWidth, workspacePanelOverlay, workspacePanelRenderable,
    workspacePanelGridOpen, sidebarRenderWidth, terminalRenderHeight,
  } = shellGeometry;

  const conversationView = projectConversation({ local: state,
    tab: activeTab, activeTabId, backgroundRuntimes, connectingLabel: t("status.connecting") });
  const visibleRuntimeState = conversationView.runtime;
  const chatSurfaceVisible = true;
  const { dockVisible: surfaceWorkspacePanelRenderable, dockGridOpen: surfaceWorkspacePanelGridOpen,
    dockOverlay: surfaceWorkspacePanelOverlay,
    terminalOpen: terminalSurfaceOpen } = projectConversationLayout({
    chatVisible: chatSurfaceVisible,
    dockRenderable: workspacePanelRenderable, dockGridOpen: workspacePanelGridOpen, dockOverlay: workspacePanelOverlay,
    dockOpen: workspacePanelOpen, dockMaximized: workspacePanelMaximized, terminalOpen: terminalPanelOpen,
  });
  const statusBarVisible = chatSurfaceVisible;
  const composerSessionKey = useMemo(() => {
    return composerDraftKeyForTab(activeTab, activeTabId);
  }, [activeTab, activeTabId]);
  const transcriptGeometrySessionKey = activeSessionIdentity;
  const workspaceScopeKey = projectWorkspaceScopeKey({
    activeTabId, sessionKey: activeSessionIdentity,
    cwd: state.meta?.cwd, sessionGen: state.sessionGen, workspaceControllerEpoch,
  });
  const workspaceTreeMemoryKey = projectWorkspaceTreeMemoryKey({
    scope: activeTab?.scope, workspaceRoot: activeTab?.workspaceRoot, cwd: state.meta?.cwd,
  });
  const { activeTopicTurns } = useSessionSummary({ activeTab, revision: projectRevision });
  const visibleUserTurns = visibleRuntimeState.items.reduce((count, item) => (item.kind === "user" ? count + 1 : count), 0);
  const currentTabTurns = Math.max(visibleRuntimeState.checkpoints.length, visibleUserTurns);
  const sessionTurns = currentTabTurns > 0 ? currentTabTurns : activeTopicTurns ?? 0;
  const startupSplashHold = !activeTabId && state.meta?.ready !== true && !state.meta?.startupErr;
  const profileProjection = useComposerProfileProjection({
    activeTabId,
    activeTab,
    meta: state.meta,
    profilesByTab: composerProfilesByTab,
    setProfilesByTab: setComposerProfilesByTab,
    tabMetas,
    planIntentsRef: userPlanModeByTabRef,
  });
  const {
    composerProfile, goal, collaborationMode, toolApprovalMode,
    patchComposerProfileForTab, patchActivatedGoalForTab,
  } = profileProjection;
  const controllerReady =
    state.meta?.ready === true &&
    (!state.meta.runtime || state.meta.runtime.phase === "ready") &&
    !state.meta.startupErr &&
    !state.backendActivationPending &&
    !runtimeTransitioning;
  useEffect(() => {
    if (controllerReady && activeTabId) noteNavigationComposerEnabled(activeTabId);
  }, [activeTabId, controllerReady]);
  useAppDiagnostics({ activeTabId, tabCount: tabMetas.length, ready: controllerReady, running: state.running,
    hydrating: state.hydrating, runtimeTransitioning, contentRevision: state.historyLayoutRevision });

  const tabBarCommands = useTabBarCommands({
    activeTabId,
    tabMetas,
    deliveryWorktreeRoot: state.meta?.workspaceRoot || state.meta?.workspacePath || state.meta?.cwd,
    t,
    showToast,
    setTabMetas,
    setTabOrderIds,
    setComposerProfilesByTab,
    setTabRevealSignal,
    clearWorkspaceConflict: () => setWorkspaceConflict(null),
    ports: {
      closeTab,
      reorderTabs,
      switchTab,
      refreshTabMetas: (apply, options) => refreshTabMetas(apply, options),
      refreshBackgroundRuntimes,
      cancelActive: () => void handleCancelActive(),
      noteNavigationIntent,
      beginNavigationSurface,
      settleNavigationSurface,
      isNavigationIntentCurrent,
      reassertVisibleTabAfterStaleNavigation,
      enterChatView: enterConversation,
      createIsolatedWorktree,
    },
  });
  const { pendingClose, setPendingClose } = tabBarCommands;

  const decisionSurface = useMemo((): AppDecisionSurfaceKind | null => projectDecisionSurface({
    approval: state.approval, ask: state.ask, mcpInteraction: state.mcpInteraction, extensionForm: state.extensionForm,
    workspaceConflict, pendingClose, clearContextPending,
  }), [clearContextPending, pendingClose, state.approval, state.ask, state.extensionForm, state.mcpInteraction, workspaceConflict]);
  const visibleDecisionSurface = decisionSurface;
  // Navigation used to hide the entire composer until the controller/runtime
  // activation ticket and the transcript paint ticket both settled.  That made
  // a local session switch look like a frozen blank surface even though its
  // history was already available (or could be shown from the bounded
  // transcript cache).  Keep the composer mounted during a local transition:
  // the submission resources still use `controllerReady` as the write fence,
  // so drafts remain editable while send/control actions stay disabled until
  // the target runtime is ready.  Decision surfaces remain exclusive.
  const composerSurfaceHidden = Boolean(decisionSurface);
  useDecisionSurfaceFocus({ surface: decisionSurface, activeTabId, closeOverlays: closeTransientOverlays });

  // Extension form surface (stage 8b2): submit delivers the structured values
  // to the owning sidecar; cancel reports values{"cancelled": true} over the
  // same channel. A failed cancel still dismisses — the sidecar that could not
  // be reached is gone either way.
  const extensionSurface = useExtensionSurface({
    activeTabId,
    hostId: state.meta?.session?.hostId,
    sessionId: state.meta?.sessionId,
    sessionGeneration: state.meta?.sessionGeneration,
    form: state.extensionForm,
    notifications: state.extensionNotifications,
    dismissForm: dismissExtensionForm,
    drainNotifications: drainExtensionNotifications,
    showToast,
  });
  const extensionStatusList = useMemo(() => Object.values(state.extensionStatuses ?? {}), [state.extensionStatuses]);
  const visibleTabId = activeTabId;
  const visibleTabs = useMemo(() => projectVisibleTabs({
    tabs: tabMetas, orderIds: tabOrderIds, profiles: composerProfilesByTab, visibleTabId, running: state.running,
  }), [composerProfilesByTab, state.running, tabMetas, tabOrderIds, visibleTabId]);

  useTabProjectionLifecycle({
    tabs: tabMetas, activeTabId, activeMeta: activeTab, meta: state.meta,
    planIntentsRef: userPlanModeByTabRef,
    setOrder: setTabOrderIds, setProfiles: setComposerProfilesByTab,
  });


  const controllerProfiles = projectControllerProfiles(tabMetas, composerProfilesByTab, {
    target: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity }, profile: composerProfile,
  });
  const controllerProfileCommands = useControllerProfileCommands({
    target: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity }, profiles: controllerProfiles,
    ready: controllerReady, runtimeEpoch: state.meta?.runtime?.epoch, operations: sessionOperations,
    ports: { model: setModelForTab, profile: setControllerComposerProfileForTab },
    report: handleGoalActionError,
  });
  const { switchModel, applyProfile: applyControllerProfile } = controllerProfileCommands;
  const hydratePlaceholderActive = Boolean(
    state.hydrating &&
    state.items.length === 0 &&
    state.hydratePlaceholderItems?.length,
  );
  const sessionUndoCommands = useSessionUndo({
    activeTabId,
    activeTabReadOnly: Boolean(activeTab?.readOnly),
    items: state.items,
    hydratePlaceholderActive,
    controllerReady, running: state.running, messageActionOpen: state.messageAction != null,
    approvalOpen: state.approval != null, askOpen: state.ask != null, clearContextPending,
    ports: {
      rewindForTab, rewindForTabDetailed, forkTurnForTab,
      refreshTabMetas: () => void refreshTabMetas(undefined, { afterMutation: true }),
      undoRewindForTab, sendToTab,
      composeInsert: replaceComposerInsert,
      refreshDock: () => setDockRefreshKey((value) => value + 1),
      refreshProject: () => setProjectRevision((value) => value + 1),
    },
  });
  const {
    rewindState, rewindCommitting, rewindSignal, setRewindStateForTab,
    handleSessionRevertCommitted, handleMessageAction, handleForkTurn, handleUndoRewind, handleEditPrompt,
  } = sessionUndoCommands;
  const clearSubmissionUndo = useCommittedCommand((tab: string) => setRewindStateForTab(tab, null));
  const { commitThenSend, submit: submitComposerTurn, applyGoal, sendRevision } = useSessionSubmission({
    target: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity }, operations: sessionOperations,
    resources: projectSubmissionResources(controllerProfiles, tabMetas, composerProfilesByTab,
      { tabId: activeTabId ?? "", profile: composerProfile, ready: controllerReady },
      { starting: t("composer.workspaceStarting"), readOnly: t("composer.readOnlyChannel") }),
    missingSource: t("composer.workspaceStarting"),
    ports: createSubmissionPorts({ send: sendToTab, setGoal: setControllerGoalForTab, clearGoal: clearControllerGoalForTab,
      clearUndo: clearSubmissionUndo, patchGoal: patchActivatedGoalForTab, profile: applyControllerProfile }),
  });
  const patchPlanExitProfileForTab = useCommittedCommand((tabId: string, mode: CollaborationMode) => {
    patchComposerProfileForTab(tabId, {
      collaborationMode: mode,
      goalDraftMode: false,
      goal: "",
    }, ["collaborationMode", "goal"]);
  });
  const modeActions = useComposerModeActions({
    target: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity },
    collaborationMode, toolApprovalMode, goal,
    operations: sessionOperations,
    planIntentsRef: userPlanModeByTabRef,
    ports: {
      setMode: setControllerModeForTab, setCollaboration: setControllerCollaborationModeForTab,
      setApproval: setToolApprovalModeForTab, clearGoal: clearControllerGoalForTab,
      patch: patchComposerProfileForTab,
    },
    showError: (message) => showToast(message, "error"),
  });
  const { applyCollaborationMode, notePlanModeForTab } = modeActions;
  const rememberPlanRevisionForTab = usePendingPlanRevisions({
    visible: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity },
    resources: controllerProfiles.map(resource => resource.target), running: state.running,
    ready: controllerReady && !state.approval && !state.ask && !state.mcpInteraction,
    operations: sessionOperations, send: sendRevision, report: reportPendingRevisionFailure,
  });
  const promptCommands = useSessionPromptCommands({
    target: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity },
    session: state.meta?.session,
    sessionGeneration: state.meta?.sessionGeneration,
    approval: state.approval,
    question: state.ask,
    mcpInteraction: state.mcpInteraction,
    goal, toolApprovalMode,
    operations: sessionOperations,
    ports: {
      approveForTab, isPromptCurrentForTab, resolvePlanForTab: resolvePlanDecisionForTab,
      answerQuestionForTab, answerMCPForTab: answerMCPInteractionForTab,
      setCollaborationModeForTab: setControllerCollaborationModeForTab,
      clearGoalForTab: clearControllerGoalForTab,
      patchComposerProfile: patchPlanExitProfileForTab, notePlanMode: notePlanModeForTab,
      rememberRevision: rememberPlanRevisionForTab,
    },
    reportError: error => showToast(error instanceof Error ? error.message : String(error), "error"),
  });
  const controlCommands = useSessionControlCommands({
    activeTabId,
    resources: controllerProfiles.map(resource => resource.target),
    operations: sessionOperations,
    showToast,
    clearWorkspaceConflict: () => setWorkspaceConflict(null),
    ports: {
      cancel,
      cancelForTab,
      acceptDelivery: (tabId) => desktopBridge.acceptDeliveryToTab(tabId),
      cancelJobForTab: (tabId, jobId) => desktopBridge.cancelJobForTab(tabId, jobId),
      refreshBackgroundRuntimes,
    },
  });
  const { handleCancelActive } = controlCommands;
  // Shift+Tab toggles only the collaboration axis. Permission presets are
  // changed explicitly through the composer permission selector.
  const cycleMode = useCommittedCommand(() => {
    runGoalAction(() => applyCollaborationMode(collaborationMode === "plan" ? "normal" : "plan"));
  });

  const todoPanelCommands = useTodoPanelCommands({
    items: visibleRuntimeState.items,
    running: visibleRuntimeState.running,
    pendingPrompt: visibleRuntimeState.pendingPrompt,
    meta: state.meta,
    activeTab,
    activeTabId,
    controllerReady,
    sessionKey: activeSessionIdentity,
    operations: sessionOperations,
    t,
    ports: {
      sendToTab: (tabId, text) => sendToTab(tabId, text),
    },
  });
  const { showTodos, scopedTodoBatch, todos, dismissTodos, handleTodoContinue } = todoPanelCommands;

  const sessionTitle = topicTitle(activeTab);
  const exportItems = state.items;
  const exportLive = liveStore.getSnapshot(activeTabId) ?? state.live;
  const sessionHasContent = exportItems.length > 0 || Boolean(exportLive?.text || exportLive?.reasoning);

  const sessionExportCommands = useSessionExportCommands({
    selector: activeTab?.session?.sessionId ? { ref: activeTab.session } : activeTab?.sessionPath ? { sessionPath: activeTab.sessionPath } : { topicId: activeTab?.topicId },
    tabId: activeTabId,
    sessionTitle,
    items: exportItems,
    live: exportLive,
    hasContent: sessionHasContent,
    t,
    showToast,
  });

  useActiveTabUiReset({ activeTabId, setClearPending: setClearContextPending, setInsertTarget: setWorkspaceInsertTarget });

  const routerCommands = useComposerRouter({
    activeTabId,
    goalDraftActive: collaborationMode === "goal" && !goal.trim(),
    t,
    notice,
    showToast,
    ports: {
      runShellForTab,
      switchModel: (name, tabId) => switchModel(name, tabId),
      newSession: () => newSession(),
      setSettingsTarget: (tab) => setSettingsTarget(tab),
      setClearContextPending,
      clearWorkspaceConflict: () => setWorkspaceConflict(null),
      setWorkspaceConflict: (value) => setWorkspaceConflict(value),
      setPendingClose: (value) => setPendingClose(value),
      submitComposerTurn: (tab, display, submit, structured, submissionId) => submitComposerTurn(tab, display, submit, structured, submissionId),
    },
  });

  const editGoal = async (objective: string, maxGoalRounds: number | null) => {
    if (!activeTabId) return;
    await editControllerGoalForTab(activeTabId, objective, maxGoalRounds);
  };
  const goalCommands = useComposerGoalCommands({ applyCollaborationMode, applyGoal, editGoal });

  const goalTarget = { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity };
  const runComposerRuntime = useCommittedCommand(async (action: "pause" | "resume" | "effort", level?: string) => {
    const outcome = await sessionOperations(goalTarget, action === "effort" ? "effort" : "goal-lifecycle", { tabId: goalTarget.tabId, action, level },
      async (operationInput, authority) => {
        authority.checkpoint();
        if (operationInput.action === "pause") await pauseControllerGoalForTab(operationInput.tabId);
        else if (operationInput.action === "resume") await resumeControllerGoalForTab(operationInput.tabId);
        else await setEffortForTab(operationInput.tabId, operationInput.level ?? "");
      });
    if (outcome.status === "failed") throw outcome.error;
  });
  const reportGoalRuntimeError = useCommittedCommand((error: unknown) => showToast(error instanceof Error ? error.message : String(error), "error"));
  const goalActions = {
    pauseGoal: useCommittedCommand(() => runGoalAction(() => runComposerRuntime("pause"))),
    resumeGoal: useCommittedCommand(() => runGoalAction(() => runComposerRuntime("resume"))),
    setEffort: useCommittedCommand((level: string) => { void runComposerRuntime("effort", level).catch(reportGoalRuntimeError); }),
  };

  const {
    refreshTabMetas, seedActiveTabMeta,
    handleRuntimeEvent, handleRuntimeReady, handleRuntimeRebuilt,
  } = useRuntimeEventHandlers({
    activeTabId,
    workspaceScopeKey,
    workspaceScopeActiveTabRef,
    userPlanModeByTabRef,
    setTabMetas,
    setTabOrderIds,
    setComposerProfilesByTab,
    setDockRefreshKey,
    setProjectRevision,
    setWorkspaceControllerEpoch,
    setControllerCollaborationMode,
  });

  const refreshProviderSetupState = useCommittedCommand(() => probeProviderSetupState());

  const leaseBlockedTab = activeLeaseBlockedTab(tabMetas, activeTab?.id ?? activeTabId);
  const bannerCommands = useSessionBannerCommands({
    reloadConfigWarnings,
  });

  const workspacePanelCommands = useWorkspacePanelCommands({
    sessionId: activeTabId ?? "",
    workspaceRoot: activeTab?.workspaceRoot ?? state.meta?.cwd ?? "",
    globalWorkspace: !activeTab || activeTab.scope === "global",
    visible: surfaceWorkspacePanelRenderable,
    closeOverlays: closeTransientOverlays, clearLiveWidth: setLiveWorkspacePanelRenderWidth,
    setTreeWidth: setRightDockTreeWidth,
    gridOpen: surfaceWorkspacePanelGridOpen,
    t,
  });
  const { openRightDockMode } = workspacePanelCommands;

  const turnVerificationCommands = useTurnVerificationCommands({
    activeTabId,
    turnStartAt: state.turnStartAt,
    completionSummary: state.completionSummary,
    sessionPath: state.meta?.sessionPath,
    openChangedDock: () => openRightDockMode("changed"),
  });

  const terminalPanelCommands = useTerminalPanelCommands({
    tabId: activeTabId, shortcutsEnabled: !managementActive,
  });

  const layoutStyle = useMemo(
    () =>
      ({
        "--sidebar-expanded-width": `${sidebarRenderWidth}px`,
        "--chat-min-width": `${chatReservedWidth}px`,
        "--workspace-width": `${workspacePanelRenderWidth}px`,
        "--workspace-resizer-width": `${WORKSPACE_RESIZER_WIDTH}px`,
        "--terminal-height": `${terminalSurfaceOpen ? liveTerminalHeight ?? terminalRenderHeight : 0}px`,
      }) as CSSProperties,
    [chatReservedWidth, liveTerminalHeight, sidebarRenderWidth, terminalRenderHeight, terminalSurfaceOpen, workspacePanelRenderWidth],
  );

  // Coalesce tab-bar switches through the same last-click-wins scheduler that
  // openTopic/blank/resume navigation uses, so rapidly clicking between two
  // running sessions can't run two switchTab() calls concurrently. Concurrent
  // switches race on the backend SetActiveTab/confirmBackendActiveTab ordering,
  const availability = projectSessionAvailability({ local: state, empty: !activeTabId && !runtimeTransitioning });
  const {
    transcriptHydrating, emptyHero,
    visibleTranscriptItems, visibleTranscriptTabId, visibleTranscriptGeometryKey,
    handleLoadOlderHistory, handleLoadNewerHistory, handleNavigateToTurn, handleSurfacePaintReady, latestGuidanceConsumed, handleTranscriptPrompt,
  } = useTranscriptSurfaceProjection({
    hydrating: state.hydrating,
    hydrateHistoryLoaded: state.hydrateHistoryLoaded,
    hydratePlaceholderItems: state.hydratePlaceholderItems,
    hydratePlaceholderActive,
    items: state.items,
    guidanceConsumed: state.guidanceConsumed,
    activeTabId,
    geometrySessionKey: transcriptGeometrySessionKey,
    controllerReady,
    availability,
    sessionActivity: Boolean(conversationView.runtime.running || conversationView.runtime.pendingPrompt
      || conversationView.runtime.approval || conversationView.runtime.ask || conversationView.runtime.extensionForm
      || conversationView.runtime.mcpInteraction),
    sessionHasContent,
    commitRendered: commitRenderedTranscriptSurface,
    commitPaint: commitNavigationSurfacePaint,
    commitSingleSurface: commitSingleSurfaceNavigation,
    ports: {
      loadOlderHistory: (tabId, targetTurn, trigger) => loadOlderHistory(tabId, targetTurn, trigger),
      loadNewerHistory: (tabId, latest, current) => loadNewerHistory(tabId, latest, current),
      navigateToTurn,
      commitThenSend: (tabId, text, submitText) => commitThenSend(tabId, text, submitText),
    },
  });

  const { openAutomationTopic, topicAccepted } = useAutomationNavigation({ noteIntent: noteNavigationIntent,
    enqueue: useCommittedCommand((intent, seq) => enqueueNavigationWithIntent(intent, seq)) });  const {
    enqueueNavigation, enqueueNavigationWithIntent,
  } = useDesktopNavigation({
    visible: { tabId: activeTabId ?? "", sessionKey: activeSessionIdentity },
    ports: { isNavigationIntentCurrent, activateTopic,
      ensureBlankSurface, openCanonicalSession, createIsolatedWorktree, openChannelSession, resumeSession,
      registeredNavigationIntent,
      listTabs: desktopBridge.listTabs, applyTabs: setTabMetas, seedTab: seedActiveTabMeta, listSessions, topicAccepted,
    },
    setTabRevealSignal, setProjectRevision, setHistory: setHistView, t, showToast,
    noteIntent: noteNavigationIntent, beginSurface: beginNavigationSurface, settleSurface: settleNavigationSurface,
    showChat: enterConversation,
  });
  return {
    insertCommands,
    clearCommands,
    tabBarCommands,
    extensionSurface,
    promptCommands,
    controlCommands,
    routerCommands,
    goalCommands,
    goalActions,
    modeActions,
    controllerProfileCommands,
    profileProjection,
    sessionExportCommands,
    workspacePanelCommands,
    turnVerificationCommands,
    terminalPanelCommands,
    bannerCommands,
    runtimeEventCommands: {
      refreshTabMetas, seedActiveTabMeta,
      handleRuntimeEvent, handleRuntimeReady, handleRuntimeRebuilt,
    },
    sessionUndo: {
      rewindState, rewindCommitting, rewindSignal, handleSessionRevertCommitted, handleMessageAction, handleForkTurn, handleUndoRewind, handleEditPrompt,
    },
    todoPanel: { showTodos, scopedTodoBatch, todos, dismissTodos, handleTodoContinue },
    transcript: {
      transcriptHydrating, emptyHero, availability,
      visibleTranscriptItems, visibleTranscriptTabId, visibleTranscriptGeometryKey,
      handleLoadOlderHistory, handleLoadNewerHistory, handleNavigateToTurn, handleSurfacePaintReady, latestGuidanceConsumed, handleTranscriptPrompt,
    },
    automation: { openAutomationTopic },
    desktopNavigation: { enqueueNavigation, enqueueNavigationWithIntent },
    invocation: { invocationMetadataByTab, handleInvocationMetadataChange },
    sessionHasContent,
    conversationView,
    visibleRuntimeState,
    surfaceWorkspacePanelRenderable,
    surfaceWorkspacePanelGridOpen,
    surfaceWorkspacePanelOverlay,
    terminalSurfaceOpen,
    statusBarVisible,
    chatSurfaceVisible,
    composerSessionKey,
    workspaceScopeKey,
    workspaceTreeMemoryKey,
    sessionTurns,
    startupSplashHold,
    controllerReady,
    decisionSurface,
    visibleDecisionSurface,
    composerSurfaceHidden,
    extensionStatusList,
    visibleTabs,
    visibleTabId,
    hydratePlaceholderActive,
    leaseBlockedTab,
    layoutStyle,
    cycleMode,
    closeTransientOverlays,
    refreshProviderSetupState,
    shellGeometry,
    appRef,
    layoutRef,
    footerHeight,
    footerRef,
    backgroundRuntimes,
    workspaceConflict,
  };
}
