import { app } from "../lib/bridge";

/** Runtime-only bridge ports used by App owners; presentation never imports the desktop bridge directly. */
export const desktopBridge = {
  getSessionSummary: (ref: Parameters<typeof app.GetSessionSummary>[0]) => app.GetSessionSummary(ref),
  cancelJobForTab: (tabId: string, jobId: string) => app.CancelJobForTab(tabId, jobId),
  terminalOutputForTab: (tabId: string, sessionId: string) => app.TerminalOutputForTab(tabId, sessionId),
  acceptDeliveryToTab: (tabId: string) => app.AcceptDeliveryToTab(tabId),
  listTabs: app.ListTabs,
  openTaskSessionForTab: app.OpenTaskSessionForTab,
  listSessionsForTab: app.ListSessionsForTab,
  closeMergedWorktreeTab: app.CloseMergedWorktreeTab,
  finalizeWorktreeMerge: app.FinalizeWorktreeMerge,
};
