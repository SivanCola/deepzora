import { useEffect, useRef, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { memberSettings } from "../lib/teamAgentSettings";
import { useToast } from "../lib/toast";
import { normalizeToolApprovalMode, type ToolApprovalMode } from "../lib/types";
import { notifyTeamChanged, useTeam } from "../lib/useTeam";
import { PermissionPresetChoice } from "./PermissionPresetChoice";

export default function TeamPermissionChoice({ agentId, disabled, dismissSignal }: {
  agentId: string; disabled: boolean; dismissSignal?: number;
}) {
  const t = useT();
  const { showToast } = useToast();
  const team = useTeam();
  const member = team.view?.roster.agents.find(candidate => candidate.id === agentId);
  const revision = team.view?.roster.revision;
  const [saving, setSaving] = useState(false);
  const pending = useRef(false);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);

  const choose = async (permission: ToolApprovalMode) => {
    if (disabled || pending.current || team.loading || !team.view?.writable || !member || revision === undefined) return;
    pending.current = true;
    setSaving(true);
    try {
      const result = await app.SaveTeamAgent({ id: agentId, expectedRevision: revision,
        requestId: crypto.randomUUID(), settings: { ...memberSettings(member), permission } });
      if (result.outcome !== "applied") {
        throw new Error(result.reason === "conflict" ? t("team.permissionConflict") : result.message || result.reason);
      }
      notifyTeamChanged();
      if (mounted.current) showToast(t("team.saved"));
    } catch (error) {
      if (mounted.current) showToast(error instanceof Error ? error.message : String(error), "error");
    } finally {
      // Never retry a stale configuration or silently replace another edit.
      if (mounted.current) { await team.reload(); setSaving(false); }
      pending.current = false;
    }
  };

  return <span title={t("team.permissionScope")}>
    <PermissionPresetChoice value={normalizeToolApprovalMode(member?.permission)}
      disabled={disabled || saving || team.loading || !team.view?.writable || !member}
      dismissSignal={dismissSignal} scopeKey={`team:${agentId}:${revision}`}
      projectConfirmationKey=""
      confirmationDescription={t("team.permissionFullAccess")}
      confirmationAcknowledge={t("team.permissionAcknowledge")}
      onPick={permission => { void choose(permission); }} />
  </span>;
}
