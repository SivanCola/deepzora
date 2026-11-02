import { FileDiff } from "lucide-react";
import type { RefObject } from "react";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import { useT } from "../lib/i18n";

export function WorkspaceChangesBadge({ tabId, scopeKey, onOpenChanges, expanded, buttonRef }: {
  tabId: string;
  scopeKey: string;
  onOpenChanges: () => void;
  expanded?: boolean;
  buttonRef?: RefObject<HTMLButtonElement | null>;
}) {
  const t = useT();
  const { workspaceChanges: changes, workspaceChangesErr } = useWorkspaceChangesResource(tabId, scopeKey, 0);
  const fileCount = changes?.files.filter(file => file.sources.includes("git")).length ?? 0;
  const added = changes?.added ?? 0;
  const removed = changes?.removed ?? 0;
  const approximate = Boolean(changes?.incomplete || workspaceChangesErr || changes?.gitErr || !changes?.gitAvailable);
  if (!changes || (fileCount === 0 && added === 0 && removed === 0 && !approximate)) return null;
  if (!changes.gitAvailable && fileCount === 0 && added === 0 && removed === 0) return null;

  return <div className="workspace-changes-bar">
    <button type="button" className="workspace-changes-badge" onClick={onOpenChanges} aria-expanded={expanded} ref={buttonRef}
      title={t(approximate ? "workspace.changeSummaryIncomplete" : "workspace.changeSummaryHint")}
      aria-label={`${t("workspace.changeSummary")}: ${fileCount} ${t("workspace.filesTab")}, ${approximate ? "~ " : ""}+${added} −${removed}`}>
      <FileDiff size={15} aria-hidden="true" />
      <span>{t("workspace.changeSummary")}</span>
      {approximate && <span className="workspace-changes-badge__estimate" aria-hidden="true">~</span>}
      <span className="workspace-changes-badge__added">+{added.toLocaleString()}</span>
      <span className="workspace-changes-badge__removed">−{removed.toLocaleString()}</span>
    </button>
  </div>;
}
