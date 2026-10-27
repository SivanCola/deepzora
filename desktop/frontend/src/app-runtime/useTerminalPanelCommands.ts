import { useCommittedCommand } from "../lib/useCommittedCommand";
import { useGlobalShortcut } from "../lib/keyboardShortcuts";
import { useLayoutStore, saveTerminalPanelOpen } from "../store/layout";
import { useTerminalStore } from "../store/terminal";

function showTerminal() {
  useLayoutStore.getState().setTerminalPanelOpen(true);
  saveTerminalPanelOpen(true);
}

/** Terminal panel commands and their global shortcuts. */
export function useTerminalPanelCommands(input: { tabId?: string; shortcutsEnabled?: boolean }) {
  const toggleTerminalPanel = useCommittedCommand(() => {
    const next = !useLayoutStore.getState().terminalPanelOpen;
    useLayoutStore.getState().setTerminalPanelOpen(next);
    saveTerminalPanelOpen(next);
  });
  const openTerminalForPath = useCommittedCommand((path = ".") => {
    showTerminal();
    if (input.tabId) void useTerminalStore.getState().createSession(input.tabId, path || ".", "default").catch(() => {});
  });
  const newTerminalSession = useCommittedCommand(() => {
    if (input.tabId) openTerminalForPath();
  });
  const closeTerminalPanel = useCommittedCommand(() => {
    useLayoutStore.getState().setTerminalPanelOpen(false);
    saveTerminalPanelOpen(false);
  });
  useGlobalShortcut("terminal.toggle", toggleTerminalPanel, [toggleTerminalPanel], input.shortcutsEnabled !== false);
  useGlobalShortcut("terminal.newSession", newTerminalSession, [newTerminalSession], input.shortcutsEnabled !== false);
  return { toggleTerminalPanel, openTerminalForPath, closeTerminalPanel };
}
