import { useEffect, useState } from "react";
import type { SaveTeamSecretRequest, TeamSecretMetadata, TeamSecretRequest, TeamView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { notifyTeamChanged } from "../lib/useTeam";
import { useTeamSecrets } from "../lib/useTeamSecrets";
import { useTeamOperation } from "../lib/useTeamOperation";
import { SettingsFeedback, useSettingsDraft } from "./SettingsExperience";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { InlineConfirmButton } from "./InlineConfirmButton";

export function TeamSecretSettings({team}:{team:TeamView}){
  const t=useT(),secrets=useTeamSecrets(),operation=useTeamOperation();
  const [editing,setEditing]=useState<{secret:TeamSecretMetadata;request?:TeamSecretRequest;revision:number;rosterRevision:number}|null>(null);
  if(!secrets.view?.available)return null;
  const view=secrets.view;
  const begin=(secret:TeamSecretMetadata,request?:TeamSecretRequest)=>setEditing({secret,request,revision:view.revision,rosterRevision:view.rosterRevision});
  const done=async()=>{setEditing(null);await secrets.reload();notifyTeamChanged();};
  const remove=(secret:TeamSecretMetadata)=>operation.run({name:secret.name,revision:view.revision},requestId=>app.SaveTeamSecret({expectedRevision:view.revision,expectedRosterRevision:view.rosterRevision,requestId,operation:"delete",name:secret.name,purpose:"",value:"",agents:[],secretRequestId:"",confirmForget:false}),done);
  return <SettingsSection title={t("team.secrets")} description={t("team.secretStorageHint")}>
    {view.pendingCleanup>0&&<SettingsFeedback kind="info">{t("team.secretCleanupPending")}</SettingsFeedback>}
    {(secrets.error||operation.error)&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):t("team.secretFailed")}</SettingsFeedback>}
    {editing?<TeamSecretEditor key={`${editing.secret.name}:${editing.revision}`} initial={editing.secret} request={editing.request} revision={editing.revision} rosterRevision={editing.rosterRevision} team={team} onSaved={done} onCancel={()=>setEditing(null)}/>:<>
      <button className="btn btn--small" type="button" disabled={!team.writable} onClick={()=>begin({name:"",purpose:"",agents:[],updatedAt:new Date().toISOString()})}>{t("team.secretAdd")}</button>
      {view.requests.map(request=><article className="cap-skill-card" key={request.id}><h3>{request.name}</h3><p>{team.roster.agents.find(a=>a.id===request.agent)?.name??request.agent} · {request.purpose}</p><button className="btn" type="button" disabled={!team.writable} onClick={()=>begin({name:request.name,purpose:request.purpose,agents:[request.agent],updatedAt:request.at},request)}>{t("team.secretRespond")}</button></article>)}
      {view.secrets.map(secret=><article className="cap-skill-card" key={secret.name}><h3>{secret.name}</h3><p>{secret.purpose}</p><p>{secret.agents.map(id=>team.roster.agents.find(a=>a.id===id)?.name??id).join(", ")||t("team.secretNobody")}</p><div className="agent-preset-row__actions"><button className="btn" type="button" disabled={!team.writable} onClick={()=>begin(secret)}>{t("common.edit")}</button><InlineConfirmButton label={t("common.delete")} confirmLabel={t("common.delete")} cancelLabel={t("common.cancel")} danger disabled={!team.writable||operation.busy} onConfirm={async()=>{await remove(secret);}}/></div></article>)}
      {view.retiredCount>0&&<InlineConfirmButton label={t("team.secretForget")} confirmLabel={t("team.secretForgetConfirm")} cancelLabel={t("common.cancel")} danger disabled={!team.writable||operation.busy} onConfirm={async()=>{await operation.run({revision:view.revision,forget:true},requestId=>app.SaveTeamSecret({expectedRevision:view.revision,expectedRosterRevision:view.rosterRevision,requestId,operation:"forget_retired",name:"",purpose:"",value:"",agents:[],secretRequestId:"",confirmForget:true}),done);}}/>}
    </>}
  </SettingsSection>;
}

export function TeamSecretRequestCards({tabID,onManage}:{tabID:string;onManage:()=>void}){
  const t=useT(),secrets=useTeamSecrets();
  const [agent,setAgent]=useState("");
  useEffect(()=>{let active=true;setAgent("");void app.TeamSecretSeatForTab(tabID).then(seat=>{if(active&&!seat.groupId)setAgent(seat.agentId);},()=>{});return()=>{active=false;};},[tabID]);
  if(!agent||!secrets.view?.available)return null;
  const requests=secrets.view.requests.filter(request=>request.agent===agent);
  return requests.length?<aside aria-label={t("team.secretRequests")}>{requests.map(request=><article className="cap-skill-card" key={request.id}><h3>{request.name}</h3><p>{request.purpose}</p><button className="btn" type="button" onClick={onManage}>{t("team.secretRespond")}</button></article>)}</aside>:null;
}

function TeamSecretEditor({initial,request,revision,rosterRevision,team,onSaved,onCancel}:{initial:TeamSecretMetadata;request?:TeamSecretRequest;revision:number;rosterRevision:number;team:TeamView;onSaved:()=>Promise<void>;onCancel:()=>void}){
  const t=useT(),operation=useTeamOperation();
  const [draft,setDraft]=useState(initial),[value,setValue]=useState("");
  const existing=team.writable&&initial.name!==""&&!request;
  const valid=/^[A-Z][A-Z0-9_]{0,39}$/.test(draft.name)&&(value.length>=4||existing);
  const dirty=value!==""||JSON.stringify(draft)!==JSON.stringify(initial);
  const submit=(decline=false)=>{
    const input:Omit<SaveTeamSecretRequest,"requestId">={expectedRevision:revision,expectedRosterRevision:rosterRevision,operation:decline?"decline":value?"save":"authorize",name:draft.name,purpose:draft.purpose,value,agents:draft.agents,secretRequestId:request?.id??"",confirmForget:false};
    return operation.run(input,requestId=>app.SaveTeamSecret({...input,requestId}),async()=>{setValue("");await onSaved();});
  };
  const guard=useSettingsDraft({label:t("team.secrets"),order:5,dirty,busy:operation.busy,canSave:valid,save:()=>submit(),discard:()=>{setDraft(initial);setValue("");}});
  return <form aria-busy={operation.busy} onSubmit={e=>{e.preventDefault();if(valid)void submit();}} autoComplete="off">
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):t("team.secretFailed")}</SettingsFeedback>}
    <SettingsField label={t("team.secretName")}><input className="mem-input" value={draft.name} readOnly={Boolean(initial.name)} maxLength={40} onChange={e=>setDraft({...draft,name:e.target.value.toUpperCase()})}/></SettingsField>
    <SettingsField label={t("team.secretPurpose")}><input className="mem-input set-grow" value={draft.purpose} maxLength={200} onChange={e=>setDraft({...draft,purpose:e.target.value})}/></SettingsField>
    <SettingsField label={t("team.secretValue")} hint={t("team.secretValueHint")}><input className="mem-input set-grow" type="password" autoComplete="new-password" value={value} maxLength={16384} onChange={e=>setValue(e.target.value)}/></SettingsField>
    <SettingsField label={t("team.secretMembers")} hint={t("team.secretMembersHint")}><div>{team.roster.agents.map(member=><label className="agent-preset-modes__option" key={member.id}><input type="checkbox" checked={draft.agents.includes(member.id)} onChange={e=>setDraft({...draft,agents:e.target.checked?[...draft.agents,member.id]:draft.agents.filter(id=>id!==member.id)})}/>{member.name}</label>)}</div></SettingsField>
    <div className="settings-save-bar"><button className="btn" type="button" disabled={operation.busy} onClick={()=>guard.leave(onCancel)}>{t("common.cancel")}</button>{request&&<button className="btn" type="button" disabled={operation.busy} onClick={()=>void submit(true)}>{t("team.secretDecline")}</button>}<button className="btn btn--primary" disabled={!team.writable||!valid||operation.busy}>{t("common.save")}</button></div>
  </form>;
}
