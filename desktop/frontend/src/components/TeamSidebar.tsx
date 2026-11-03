import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Users, Settings } from "lucide-react";
import { useT } from "../lib/i18n";
import { useTeam } from "../lib/useTeam";
import { AgentPresetIcon } from "./AgentPresetIcon";
import { ErrorMessage } from "./ErrorMessage";
import { useProjectTreeReadActivity } from "./useProjectTreeReadActivity";
import { projectTreeReadActivityKey } from "../lib/projectTreeTopic";
import type { ProjectNode } from "../lib/types";
import "./Team.css";

export function TeamSidebar({activeSessionPath,activeGroup,onOpenAgent,onOpenGroup,onManage}:{activeSessionPath?:string;activeGroup?:string;onOpenAgent:(id:string)=>void;onOpenGroup:(id:string)=>void;onManage:()=>void}) {
  const t=useT();
  const {view,loading,error,reload}=useTeam();
  const [expanded,setExpanded]=useState(true);
  const activityNodes=useMemo<ProjectNode[]>(()=>view?.agentActivity.map(activity=>({key:activity.agentId,kind:"topic",label:activity.agentId,session:{hostId:"local",sessionId:activity.sessionId},sessionPath:`session-id:${activity.sessionId}`,resultSequence:activity.resultSequence,turnsState:activity.ready?"ready":"pending",sortOrder:0}))??[],[view]);
  const {readActivity,markNodeRead}=useProjectTreeReadActivity(activityNodes,false);
  useEffect(()=>{
    if(activeGroup||document.visibilityState!=="visible")return;
    const node=activityNodes.find(node=>node.sessionPath===activeSessionPath);if(node)markNodeRead(node);
  },[activityNodes,activeGroup,activeSessionPath,markNodeRead]);
  const members=[...(view?.roster.agents??[])].filter(a=>!a.hidden).sort((a,b)=>Number(!!b.pinned)-Number(!!a.pinned));
  const groups=[...(view?.roster.groups??[])].filter(g=>!g.hidden&&!g.archived).sort((a,b)=>Number(!!b.pinned)-Number(!!a.pinned));
  return <section className="team-sidebar" aria-label={t("settings.tab.team")}>
    <div className="team-sidebar__heading"><button type="button" onClick={()=>setExpanded(value=>!value)} aria-expanded={expanded}>{expanded?<ChevronDown size={14}/>:<ChevronRight size={14}/>}<Users size={15}/><span>{t("settings.tab.team")}</span></button>
      <button type="button" aria-label={t("team.manage")} onClick={onManage}><Settings size={14}/></button></div>
    {expanded&&<div className="team-sidebar__list">
      {error&&<div role="alert"><ErrorMessage error={error}/><button type="button" onClick={()=>void reload()}>{t("common.retry")}</button></div>}
      {loading&&!view&&<span role="status">{t("settings.loading")}</span>}
      {view&&!members.length&&!groups.length&&<button className="team-sidebar__empty" type="button" onClick={onManage}>{t("team.empty")}</button>}
      {members.map(member=>{
        const seats=view?.seats.filter(seat=>seat.agentId===member.id)??[];
        const status=seats.some(seat=>seat.work.pendingPrompt)?"team.approval":seats.some(seat=>seat.work.running||seat.work.jobs.length>0)?"team.running":"team.idle";
        const node=activityNodes.find(node=>node.key===member.id);
        const readKey=node?projectTreeReadActivityKey(node):null;
        const unread=!!node&&!!readKey&&node.turnsState==="ready"&&(node.resultSequence??0)>(readActivity[readKey]??0)&&(!!activeGroup||node.sessionPath!==activeSessionPath);
        return <button type="button" className="team-sidebar__row" key={member.id} aria-current={!activeGroup&&member.sessionPath===activeSessionPath?"page":undefined} onClick={()=>onOpenAgent(member.id)}>
          <AgentPresetIcon icon={member.avatar.glyph} image={member.avatar.image} color={member.avatar.color}/><span>{member.name}</span><small>{unread?`${t("team.unread")} · `:""}{t(status)}</small>
        </button>;
      })}
      {groups.map(group=>{const unread=view?.groupStatus.find(status=>status.groupId===group.id)?.unread??0;return <button type="button" className="team-sidebar__row" key={group.id} aria-current={activeGroup===group.id?"page":undefined} onClick={()=>onOpenGroup(group.id)}><Users size={16}/><span>{group.name}</span><small>{unread>0?`${unread} · ${t("team.unread")}`:group.members.length}</small></button>;})}
      {!!view&&[...view.roster.groups.filter(group=>group.archived),...view.roster.retiredGroups].length>0&&<details><summary>{t("team.history")}</summary>{[...view.roster.groups.filter(group=>group.archived),...view.roster.retiredGroups].map(group=><button type="button" className="team-sidebar__row" key={group.id} onClick={()=>onOpenGroup(group.id)}><Users size={16}/><span>{group.name}</span><small>{t("team.archived")}</small></button>)}</details>}
    </div>}
  </section>;
}
