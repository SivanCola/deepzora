import { useEffect, useLayoutEffect, useRef } from "react";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import { DOCK_ENTRIES } from "../lib/dockEntries";
import type { Translator } from "../lib/i18n";
import { loadWorkspacePanelOpen, saveWorkspacePanelOpen, useLayoutStore } from "../store/layout";
import { dockModeForTab, selectRightDockMode, useActivityBarStore, type RightDockMode, type TabType } from "../store/activityBar";

type Input = {
  sessionId: string;
  workspaceRoot: string;
  globalWorkspace?: boolean;
  visible: boolean;
  closeOverlays: () => void;
  clearLiveWidth: (width: null) => void;
  setTreeWidth: (width: number) => void;
  t: Translator;
};

function tabForDockMode(mode: RightDockMode): TabType {
  switch (mode) {
    case "context": return "context";
    case "changed": return "changed";
    case "browser": return "browser";
    default: return "file";
  }
}

// Resolve the label for each workspace tab from its entry configuration.
function labelKeyForTab(type: TabType): string {
  if (type === "browser") return "rightDock.browser";
  return DOCK_ENTRIES.find(entry => entry.defaultTab === type)?.labelKey ?? "workspace.filesTab";
}

/** One project-scoped preference owner, with no mirrored layout state. */
export function useWorkspacePanelCommands(input: Input) {
  const t = input.t;
  const defaultTabSessionRef = useRef("");
  const unboundLayoutTouched = useRef(false);
  const noteLayoutChoice = () => {
    if (input.globalWorkspace && !input.workspaceRoot) unboundLayoutTouched.current = true;
  };
  const openRightDockMode = useCommittedCommand((requestedMode?: RightDockMode) => {
    noteLayoutChoice();
    input.closeOverlays();
    const layout = useLayoutStore.getState();
    const currentMode = selectRightDockMode(useActivityBarStore.getState());
    const next = requestedMode ?? currentMode;
    if (next === "context" || next !== currentMode) layout.setWorkspacePreviewActive(false);
    layout.setWorkspacePanelMaximized(false);
    // The tab list decides what the dock renders, so every caller of this
    // command must leave a matching tab behind — otherwise the panel opens
    // empty (the tab model has no "mode" to fall back to).
    useActivityBarStore.getState().openEntry(tabForDockMode(next), t(labelKeyForTab(tabForDockMode(next)) as never));
    if (layout.workspacePanelOpen && !layout.workspacePanelMaximized) return;
    layout.setWorkspacePanelOpen(true);
    saveWorkspacePanelOpen(true, input.workspaceRoot);
  });
  const closeWorkspacePanel = useCommittedCommand(() => {
    noteLayoutChoice();
    input.closeOverlays();
    const layout = useLayoutStore.getState();
    if (!layout.workspacePanelOpen) return;
    input.clearLiveWidth(null);
    layout.setWorkspacePanelMaximized(false);
    layout.setWorkspacePanelOpen(false);
    saveWorkspacePanelOpen(false, input.workspaceRoot);
  });
  // Closing the last tab must release the dock's layout space as well as its
  // content. Observe the transition synchronously so close-all and close/open
  // in one batch use the same panel command. Project restoration is not a close.
  useEffect(() => useActivityBarStore.subscribe((state, previous) => {
    if (state.workspaceRoot !== previous.workspaceRoot || state.workspaceRoot !== input.workspaceRoot) return;
    if (previous.tabs.length > 0 && state.tabs.length === 0) closeWorkspacePanel();
  }), [input.workspaceRoot, closeWorkspacePanel]);
  const prepareBlankWorkspace = useCommittedCommand((workspaceRoot = input.workspaceRoot) => {
    unboundLayoutTouched.current = false;
    input.closeOverlays();
    input.clearLiveWidth(null);
    const layout = useLayoutStore.getState();
    layout.setWorkspacePanelMaximized(false);
    layout.setWorkspacePanelOpen(false);
    // Seed the destination preference before project restoration can run.
    saveWorkspacePanelOpen(false, workspaceRoot);
  });
  // The workspace tab picker opens the selected entry through the dock owner.
  const openDockEntry = useCommittedCommand((entryId: string) => {
    const entry = DOCK_ENTRIES.find(candidate => candidate.id === entryId);
    if (!entry) return;
    openRightDockMode(dockModeForTab(entry.defaultTab));
  });
  // Plain open/close restores the project's tabs. The session initialization
  // effect below fills an expanded empty dock with Overview once per session.
  const toggleWorkspacePanel = useCommittedCommand(() => {
    noteLayoutChoice();
    const layout = useLayoutStore.getState();
    if (layout.workspacePanelOpen) { closeWorkspacePanel(); return; }
    input.closeOverlays();
    const activity = useActivityBarStore.getState();
    if (activity.tabs.length > 0) activity.setActivityBarOpen(true);
    layout.setWorkspacePanelMaximized(false);
    layout.setWorkspacePanelOpen(true);
    saveWorkspacePanelOpen(true, input.workspaceRoot);
  });
  const toggleWorkspaceMaximized = useCommittedCommand(() => {
    input.closeOverlays();
    const layout = useLayoutStore.getState();
    layout.setWorkspacePanelMaximized(!layout.workspacePanelMaximized);
  });
  const handleWorkspacePreviewModeChange = useCommittedCommand((active: boolean) => {
    const layout = useLayoutStore.getState();
    if (layout.workspacePreviewActive === active) return;
    input.closeOverlays();
    layout.setWorkspacePreviewActive(active);
  });
  const restoreWorkspaceDockWidths = useCommittedCommand((treeWidth: number, _previewWidth: number) => {
    // Restore the preference, not its temporary narrow-window rendering.
    input.setTreeWidth(treeWidth);
  });
  useLayoutEffect(() => {
    const activity = useActivityBarStore.getState();
    // The first canonical global session binds the welcome's unbound layout
    // to its real directory. Only a choice made on this welcome may transfer;
    // ordinary project navigation and startup restoration keep their own data.
    const inheritUnbound = Boolean(unboundLayoutTouched.current && input.globalWorkspace
      && input.workspaceRoot && activity.workspaceRoot === "");
    if (inheritUnbound) saveWorkspacePanelOpen(useLayoutStore.getState().workspacePanelOpen, input.workspaceRoot);
    activity.setWorkspaceRoot(input.workspaceRoot, { inheritUnbound });
    if (input.workspaceRoot) unboundLayoutTouched.current = false;
  }, [input.workspaceRoot, input.globalWorkspace]);
  useLayoutEffect(() => {
    // Resolve the restored tab before sizing the first open dock. Overview has
    // a smaller default; an existing width remains shared across all tabs.
    const layout = useLayoutStore.getState();
    // Restore the project's open preference even for hidden surfaces, while
    // deferring first-open sizing until a visible session supplies its tab.
    layout.setWorkspacePanelOpen(loadWorkspacePanelOpen(input.workspaceRoot), Boolean(input.visible && input.sessionId));
  }, [input.workspaceRoot, input.visible, input.sessionId]);
  // A restored, expanded dock should show useful session context immediately.
  // Seed only once per session so manually reopening an emptied dock can show
  // the tab picker without immediately recreating Overview.
  useLayoutEffect(() => {
    if (!input.visible || !input.sessionId) return;
    const sessionKey = `${input.workspaceRoot}\u0000${input.sessionId}`;
    if (defaultTabSessionRef.current === sessionKey) return;
    if (!useLayoutStore.getState().workspacePanelOpen) return;
    defaultTabSessionRef.current = sessionKey;
    const activity = useActivityBarStore.getState();
    if (activity.activeTabId) return;
    activity.openEntry("context", t(labelKeyForTab("context") as never));
  }, [input.sessionId, input.visible, input.workspaceRoot, t]);
  return {
    openRightDockMode, closeWorkspacePanel, prepareBlankWorkspace, openDockEntry,
    toggleWorkspacePanel, toggleWorkspaceMaximized, handleWorkspacePreviewModeChange,
    restoreWorkspaceDockWidths,
  };
}
