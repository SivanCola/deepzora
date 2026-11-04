import { Settings } from "lucide-react";
import { AgentPresetIcon } from "./AgentPresetIcon";
import { useT } from "../lib/i18n";
import type { Item } from "../lib/useController";
import "./TeamPrivateIdentity.css";

export type TeamPrivateIdentity = NonNullable<Item["speaker"]> & { onManage: () => void };

export function TeamPrivateHeader({ member }: { member: TeamPrivateIdentity }) {
  const t = useT();
  return <div className="team-private-header">
    <AgentPresetIcon icon={member.glyph} image={member.image} color={member.color} />
    <strong>{member.name}</strong><span>{t("team.privateChat")}</span>
    <button type="button" className="chip" onClick={member.onManage} aria-label={t("team.memberSettings")}>
      <Settings size={14} /><span>{t("team.memberSettings")}</span>
    </button>
  </div>;
}
