// Icons for workspace tabs and the tab picker, kept separate from the
// synchronous entry configuration.
import { Activity, FileDiff, FileText } from "lucide-react";
import type { ComponentType } from "react";
import type { TabType } from "../store/activityBar";

export const DOCK_ENTRY_ICONS: Record<TabType, ComponentType<{ size?: number | string; className?: string }>> = {
  file: FileText,
  changed: FileDiff,
  context: Activity,
  browser: FileText,
};
