import { useMemo } from "react";
import { useTeam } from "./useTeam";
import type { Item } from "./useController";

// The caller supplies the displayed transcript's host-owned seat identity.
// Sidebar selection and a pending navigation never change its speaker.
export function useTeamSpeaker(agentID?:string):Item["speaker"] {
  const team=useTeam(Boolean(agentID));
  const member=[...(team.view?.roster.agents??[]),...(team.view?.roster.retiredAgents??[])].find(member=>member.id===agentID);
  const id=member?.id,name=member?.name,glyph=member?.avatar.glyph,image=member?.avatar.image,color=member?.avatar.color;
  return useMemo(()=>id?{id,name:name??id,glyph,image,color}:undefined,[id,name,glyph,image,color]);
}
