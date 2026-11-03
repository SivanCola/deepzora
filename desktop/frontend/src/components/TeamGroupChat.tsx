import { useCallback, useRef, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { useTeam, notifyTeamChanged } from "../lib/useTeam";
import { useTeamGroupHistory } from "../lib/useTeamGroupHistory";
import { TeamGroupProjection } from "../lib/teamGroupProjection";
import { clearSubmittedTeamDraft, flushTeamDraft, trackTeamSubmission, useTeamDraft } from "../store/teamComposer";
import { Transcript } from "./Transcript";
import { SettingsFeedback } from "./SettingsExperience";
import { ErrorMessage } from "./ErrorMessage";
import { InlineConfirmButton } from "./InlineConfirmButton";
import "./Team.css";

export function TeamGroupChat({groupID,onManage,onRevealSeat}:{groupID:string;onManage:()=>void;onRevealSeat:(tabID:string)=>void}) {
  const t=useT();
  const team=useTeam();
  const history=useTeamGroupHistory(groupID);
  const group=[...(team.view?.roster.groups??[]),...(team.view?.roster.retiredGroups??[])].find(group=>group.id===groupID);
  const composer=useTeamDraft(groupID);
  const {draft,set:setDraft}=composer;
  const [projection]=useState(()=>new TeamGroupProjection());
  const [sending,setSending]=useState(false);
  const [error,setError]=useState("");
  const sendingRef=useRef(false);
  const invalidMention=draft.mentions.some(id=>!group?.members.includes(id));
  const items=projection.update(groupID,history.lines,[...(team.view?.roster.agents??[]),...(team.view?.roster.retiredAgents??[])],t);
  const busy=team.view?.seats.some(seat=>seat.groupId===groupID&&(seat.work.running||seat.work.pendingPrompt||seat.work.jobs.length>0))??false;
  const send=useCallback(async()=>{
    if(sendingRef.current||!composer.ready||composer.error||!draft.text.trim()||!group||group.archived||invalidMention)return;
    sendingRef.current=true;setSending(true);setError("");
    try {
      await trackTeamSubmission(groupID,async()=>{
      await flushTeamDraft(groupID);
      const result=await app.SendTeamGroupMessage({groupId:groupID,requestId:draft.requestId,text:draft.text,mentions:draft.mentions,everyone:draft.everyone});
      if(result.outcome!=="accepted"){setError(result.message??result.reason??t("chat.loadFailed"));return;}
      // Clear only the submitted draft; a later edit belongs to the user.
      clearSubmittedTeamDraft(groupID,draft);
      if(result.message)setError(result.message);
      notifyTeamChanged();await history.refresh();
      await flushTeamDraft(groupID);
      });
    }catch(cause){setError(cause instanceof Error?cause.message:String(cause));}
    finally{sendingRef.current=false;setSending(false);}
  },[draft,group,groupID,invalidMention,history,composer.ready,composer.error,t]);
  const action=async(operation:()=>Promise<unknown>)=>{try{setError("");await operation();notifyTeamChanged();}catch(cause){setError(String(cause));}};
  return <section className="team-chat" aria-label={group?.name||t("team.groups")}>
    <header className="team-chat__header"><div><h1>{group?.name||t("team.groups")}</h1>{group?.notice&&<p>{group.notice}</p>}</div><div className="team-chat__actions">
      <button className="btn btn--small" type="button" onClick={()=>void action(()=>app.ContinueTeamGroup(groupID))} disabled={!team.view?.writable||group?.archived}>{t("team.continue")}</button>
      <button className="btn btn--small" type="button" onClick={()=>void action(()=>app.StopTeamGroup(groupID))} disabled={!busy}>{t("team.stop")}</button>
      <button className="btn btn--small" type="button" onClick={onManage}>{t("team.manage")}</button>
    </div></header>
    {team.error&&<SettingsFeedback kind="error" onRetry={()=>void team.reload()}>{team.error}</SettingsFeedback>}
    {team.view&&!group&&<SettingsFeedback kind="error">{t("team.groupUnavailable")}</SettingsFeedback>}
    {!!error&&<div role="alert"><ErrorMessage error={error}/></div>}
    {history.error&&<SettingsFeedback kind="error" onRetry={()=>void history.refresh()}>{history.error==="stale_cursor"?t("team.historyChanged"):history.error}</SettingsFeedback>}
    {composer.error&&<SettingsFeedback kind="error" onRetry={()=>void composer.retry().catch(()=>{})}>{composer.error.includes("conflict")?t("team.conflict"):composer.error}</SettingsFeedback>}
    {composer.error?.includes("conflict")&&<InlineConfirmButton label={t("team.reloadSaved")} confirmLabel={t("team.confirmDiscardDraft")} cancelLabel={t("common.cancel")} onConfirm={()=>composer.reload().catch(()=>{})}/>}
    <Transcript geometrySessionKey={`team-group:${groupID}`} items={items} onPrompt={()=>{}} running={false} hydrating={history.loading&&!items.length} onFollowChange={history.onFollowChange}
      hasOlderHistory={history.hasOlder} hasNewerHistory={history.hasNewer} loadingOlderHistory={history.loading} loadingNewerHistory={history.loading} onLoadOlderHistory={history.loadOlder} onLoadNewerHistory={history.loadNewer}/>
    <form className="team-chat__composer" onSubmit={event=>{event.preventDefault();void send();}}>
      <fieldset className="team-chat__mentions" disabled={sending||group?.archived||!composer.ready||!!composer.error}><legend>{t("team.mention")}</legend>
        <label><input type="checkbox" checked={draft.everyone} onChange={event=>setDraft({...draft,everyone:event.target.checked})}/>{t("team.everyone")}</label>
        {group?.members.map(id=>{const member=team.view?.roster.agents.find(member=>member.id===id);return <label key={id}><input type="checkbox" checked={draft.mentions.includes(id)} onChange={event=>setDraft({...draft,mentions:event.target.checked?[...draft.mentions,id]:draft.mentions.filter(value=>value!==id)})}/>{member?.name??id}</label>;})}
      </fieldset>
      {invalidMention&&<SettingsFeedback kind="error">{t("team.invalidMention")}<button className="btn btn--small" type="button" onClick={()=>setDraft({...draft,mentions:draft.mentions.filter(id=>group?.members.includes(id))})}>{t("common.edit")}</button></SettingsFeedback>}
      <textarea className="mem-textarea" aria-label={t("team.message")} placeholder={t("team.message")} value={draft.text} disabled={sending||group?.archived||!composer.ready||!!composer.error} onChange={event=>setDraft({...draft,text:event.target.value})} onKeyDown={event=>{if((event.metaKey||event.ctrlKey)&&event.key==="Enter"&&!event.nativeEvent.isComposing){event.preventDefault();void send();}}}/>
      <div className="team-chat__actions"><button className="btn btn--primary" type="submit" disabled={!composer.ready||!!composer.error||!draft.text.trim()||sending||invalidMention||!group||group.archived||!team.view?.writable}>{t("team.send")}</button>
        {team.view?.seats.filter(seat=>seat.groupId===groupID&&seat.work.pendingPrompt).map(seat=><button className="btn btn--small" type="button" key={seat.agentId} onClick={()=>onRevealSeat(seat.tabId)}>{team.view?.roster.agents.find(member=>member.id===seat.agentId)?.name} · {t("team.approval")}</button>)}
      </div>
    </form>
  </section>;
}
