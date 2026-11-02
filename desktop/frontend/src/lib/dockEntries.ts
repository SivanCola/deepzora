// Entry configuration for the workspace tab picker and tab labels. Kept
// icon-free so the composition layer can import it synchronously.

import type { TabType } from "../store/activityBar";

export interface DockEntryConfig {
  id: string;
  labelKey: string;
  defaultTab: TabType;
  /** Only offered when the shell exposes an embedded browser surface. */
  requiresBrowser?: boolean;
}

export const DOCK_ENTRIES: DockEntryConfig[] = [
  { id: "context", labelKey: "rightDock.overview", defaultTab: "context" },
  { id: "files", labelKey: "workspace.filesTab", defaultTab: "file" },
  { id: "changed", labelKey: "workspace.changedTab", defaultTab: "changed" },
  { id: "browser", labelKey: "rightDock.browser", defaultTab: "browser", requiresBrowser: true },
];

export function availableDockEntries(browserAvailable: boolean): DockEntryConfig[] {
  return DOCK_ENTRIES.filter((entry) => !entry.requiresBrowser || browserAvailable);
}
