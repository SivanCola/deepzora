import { useEffect, useRef, useState } from "react";
import "./AgentPresets.css";
import { ArrowUp, ArrowDown, Plus, RefreshCw } from "lucide-react";
import type { TeamAgentView as Agent, Group, TeamAgentSettings, TeamGroupSettings, TeamMutationResult, TeamPresetDefinition } from "../generated/desktopContract.generated";
import { normalizeToolApprovalMode, type ModelInfo } from "../lib/types";
import { fullAccessProjectConfirmationKey } from "../lib/fullAccessConfirmation";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { notifyTeamChanged, useTeam } from "../lib/useTeam";
import { useAgentPresets } from "../lib/useAgentPresets";
import { AgentPresetIcon } from "./AgentPresetIcon";
import { AgentPresetIconPicker } from "./AgentPresetIconPicker";
import { AgentPresetEditor, type AgentPresetDraft } from "./AgentPresetEditor";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";
import { InlineConfirmButton } from "./InlineConfirmButton";
import { PermissionPresetChoice } from "./PermissionPresetChoice";
import { TeamMemorySettings } from "./TeamMemorySettings";
import { TeamScheduleSettings } from "./TeamScheduleSettings";
import { TeamSecretSettings } from "./TeamSecretSettings";
import { TeamUsageSettings } from "./TeamUsageSettings";
import { BotChannelsSettingsPage } from "./BotChannelsSettingsPage";
import { useBotText } from "../lib/botChannels";
import { SettingsFeedback, SettingsSubpageHeader, useSettingsDraft, useSettingsExperience } from "./SettingsExperience";

type Editor = { kind:"agent"; member?:Agent; revision:number } | { kind:"group"; group?:Group; revision:number };
const memberSettings=(a?:Agent):TeamAgentSettings=>({name:a?.name??"",role:a?.role??"",instructions:a?.instructions??"",avatar:a?.avatar??{},initialModel:a?.initialModel??"",folder:a?.folder??"",permission:a?.permission||"read-only",main:a?.main??false,pinned:a?.pinned??false,hidden:a?.hidden??false,preset:a?.preset});
const groupSettings=(g?:Group):TeamGroupSettings=>({name:g?.name??"",notice:g?.notice??"",members:g?.members??[],admin:g?.admin??"",mode:g?.mode??"everyone",relayLimit:g?.relayLimit??20,folder:g?.folder??"",pinned:g?.pinned??false,hidden:g?.hidden??false,archived:g?.archived??false});
const presetCopy=(p:Omit<AgentPresetDraft,"scope">):TeamPresetDefinition=>({id:p.id,name:p.name,description:p.description,body:p.body,icon:p.icon??"",iconImage:p.iconImage??"",iconColor:p.iconColor??"",keepCoding:p.keepCoding,complete:p.complete,includeRuntimeContext:p.includeRuntimeContext,restrictTools:p.tools!==null,tools:p.tools??[]});

export function TeamSettingsPage({tabId,workspaceKey}:{tabId:string;workspaceKey:string}) {
  const t=useT();
  const team=useTeam();
  const {requestLeave}=useSettingsExperience();
  const [editor,setEditor]=useState<Editor|null>(null);
  const [notice,setNotice]=useState("");
  const [actionError,setActionError]=useState("");
  const [failedTarget,setFailedTarget]=useState({agentId:"",groupId:""});
  const revision=team.view?.roster.revision??0;
  const finish=async(result:TeamMutationResult)=>{
    await team.reload();notifyTeamChanged();
    if(result.outcome!=="applied")return false;
    setEditor(null);setNotice(t("team.saved"));return true;
  };
  const close=()=>setEditor(null);
  const operation=useTeamSave(finish);
  const reorder=(kind:"agents"|"groups",id:string,delta:number)=>{
    const roster=team.view?.roster;if(!roster)return;
    const agents=roster.agents.map(member=>member.id),groups=roster.groups.map(group=>group.id);
    const order=kind==="agents"?agents:groups,index=order.indexOf(id),next=index+delta;
    if(index<0||next<0||next>=order.length)return;
    [order[index],order[next]]=[order[next],order[index]];
    const value={agentIds:agents,groupIds:groups,expectedRevision:revision};
    void operation.save(value,requestId=>app.ReorderTeam({...value,requestId}));
  };
  const remove=(agentId:string,groupId:string)=>{setFailedTarget({agentId,groupId});return operation.save({agentId,groupId,revision},requestId=>app.DeleteTeamTarget({agentId,groupId,expectedRevision:revision,requestId}));};
  const stopTarget=async()=>{
    try {
      setActionError("");
      for(const group of team.view?.roster.groups??[])if(group.id===failedTarget.groupId||group.members.includes(failedTarget.agentId))await app.StopTeamGroup(group.id);
      if(failedTarget.agentId)await app.StopTeamSeat(failedTarget.agentId,"");
      await team.reload();
    }catch(cause){setActionError(String(cause));}
  };
  return <section aria-label={t("settings.tab.team")} aria-busy={team.loading}>
    {team.error&&<SettingsFeedback kind="error" onRetry={()=>void team.reload()}>{team.error}</SettingsFeedback>}
    {!team.view&&team.loading&&<SettingsFeedback kind="loading"/>}
    {notice&&<SettingsFeedback kind="success">{notice}</SettingsFeedback>}
    {actionError&&<SettingsFeedback kind="error">{actionError}</SettingsFeedback>}
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="busy"?t("team.busy"):operation.error==="conflict"?t("team.conflict"):operation.error}</SettingsFeedback>}
    {operation.error==="busy"&&<button className="btn" type="button" onClick={()=>void stopTarget()}>{t("team.stopTarget")}</button>}
    {team.view&&!team.view.writable&&<SettingsFeedback kind="info">{t("team.readOnly")}</SettingsFeedback>}
    {editor?.kind==="agent"?<TeamAgentEditor key={`${editor.member?.id??"new"}:${editor.revision}`} initial={memberSettings(editor.member)} memberID={editor.member?.id??""} revision={editor.revision}
      tabId={tabId} workspaceKey={workspaceKey} onSaved={finish} onCancel={close} onReload={()=>requestLeave(()=>{const member=team.view?.roster.agents.find(a=>a.id===editor.member?.id);setEditor({kind:"agent",member,revision});})}/>
    :editor?.kind==="group"?<TeamGroupEditor key={`${editor.group?.id??"new"}:${editor.revision}`} initial={groupSettings(editor.group)} groupID={editor.group?.id??""} revision={editor.revision}
      members={team.view?.roster.agents??[]} onSaved={finish} onCancel={close} onReload={()=>requestLeave(()=>{const group=team.view?.roster.groups.find(g=>g.id===editor.group?.id);setEditor({kind:"group",group,revision});})}/>
    :team.view&&<>
      <SettingsSection title={t("team.members")} description={t("team.navHint")} actions={<>
        <button className="btn btn--small" type="button" aria-label={t("summary.refresh")} disabled={team.loading} onClick={()=>void team.reload()}><RefreshCw size={14}/></button>
        <button className="btn btn--small" type="button" disabled={!team.view.writable} onClick={()=>{setNotice("");setEditor({kind:"agent",revision});}}><Plus size={14}/>{t("team.newAgent")}</button>
      </>}>
        {!team.view.roster.agents.length&&<SettingsFeedback kind="empty">{t("team.empty")}</SettingsFeedback>}
        <div className="cap-skills">{team.view.roster.agents.map((member,index)=><article className="cap-skill-card agent-preset-row" key={member.id}>
          <div className="agent-preset-row__head"><AgentPresetIcon icon={member.avatar.glyph} color={member.avatar.color} image={member.avatar.image}/><h3>{member.name}</h3>{member.main&&<span className="cap-skill-badge">{t("team.main")}</span>}</div>
          <p className="cap-skill-card__desc">{member.role}</p>
          {team.view?.seats.some(seat=>seat.agentId===member.id&&seat.settingsPending)&&<p role="status">{t("team.pendingConfig")}</p>}
          <div className="agent-preset-row__actions"><button className="btn btn--small" type="button" disabled={!team.view?.writable||operation.busy} onClick={()=>{setNotice("");setEditor({kind:"agent",member,revision});}}>{t("common.edit")}</button>
            <button className="btn btn--small" type="button" disabled={!team.view?.writable||operation.busy} onClick={()=>{setFailedTarget({agentId:member.id,groupId:""});void operation.save({agentId:member.id,revision,action:"archive"},requestId=>app.ArchiveTeamAgent({agentId:member.id,groupId:"",expectedRevision:revision,requestId}));}}>{t("team.archiveMember")}</button>
            <button className="btn btn--small" type="button" aria-label={t("team.moveUp",{name:member.name})} disabled={!team.view?.writable||operation.busy||index===0} onClick={()=>reorder("agents",member.id,-1)}><ArrowUp size={14}/></button>
            <button className="btn btn--small" type="button" aria-label={t("team.moveDown",{name:member.name})} disabled={!team.view?.writable||operation.busy||index===(team.view?.roster.agents.length??0)-1} onClick={()=>reorder("agents",member.id,1)}><ArrowDown size={14}/></button>
            <InlineConfirmButton label={t("common.delete")} confirmLabel={t("team.confirmDelete",{name:member.name})} cancelLabel={t("common.cancel")} disabled={!team.view?.writable||operation.busy} danger onConfirm={async()=>{await remove(member.id,"");}}/>
          </div>
        </article>)}</div>
      </SettingsSection>
      {team.view.roster.retiredAgents.some(member=>member.archived)&&<SettingsSection title={t("team.archivedMembers")} description={t("team.restoreMemberHint")}>
        {team.view.roster.retiredAgents.filter(member=>member.archived).map(member=><article className="cap-skill-card" key={member.id}><h3>{member.name}</h3><button className="btn btn--small" type="button" disabled={!team.view?.writable||operation.busy} onClick={()=>void operation.save({agentId:member.id,revision,action:"restore"},requestId=>app.RestoreTeamAgent({agentId:member.id,groupId:"",expectedRevision:revision,requestId}))}>{t("team.restoreMember")}</button></article>)}
      </SettingsSection>}
      <SettingsSection title={t("team.groups")} actions={<button className="btn btn--small" type="button" disabled={!team.view.writable||team.view.roster.agents.length<2} onClick={()=>{setNotice("");setEditor({kind:"group",revision});}}><Plus size={14}/>{t("team.newGroup")}</button>}>
        <div className="cap-skills">{team.view.roster.groups.map((group,index)=><article className="cap-skill-card" key={group.id}><h3>{group.name}</h3><p>{group.notice}</p><p>{group.members.map(id=>team.view?.roster.agents.find(a=>a.id===id)?.name??id).join(", ")}</p>
          {group.archived&&<span>{t("team.archived")}</span>}<div className="agent-preset-row__actions"><button className="btn btn--small" type="button" disabled={!team.view?.writable||operation.busy} onClick={()=>setEditor({kind:"group",group,revision})}>{t("common.edit")}</button>
          <button className="btn btn--small" type="button" aria-label={t("team.moveUp",{name:group.name??group.id})} disabled={!team.view?.writable||operation.busy||index===0} onClick={()=>reorder("groups",group.id,-1)}><ArrowUp size={14}/></button>
          <button className="btn btn--small" type="button" aria-label={t("team.moveDown",{name:group.name??group.id})} disabled={!team.view?.writable||operation.busy||index===(team.view?.roster.groups.length??0)-1} onClick={()=>reorder("groups",group.id,1)}><ArrowDown size={14}/></button>
          <InlineConfirmButton label={t("common.delete")} confirmLabel={t("team.confirmDelete",{name:group.name??group.id})} cancelLabel={t("common.cancel")} disabled={!team.view?.writable||operation.busy} danger onConfirm={async()=>{await remove("",group.id);}}/></div>
        </article>)}</div>
      </SettingsSection>
      <TeamMemorySettings team={team.view} reload={team.reload}/>
      <TeamScheduleSettings team={team.view}/>
      <TeamSecretSettings team={team.view}/>
      <TeamUsageSettings team={team.view}/>
    </>}
  </section>;
}

function useTeamSave(onSaved:(result:TeamMutationResult)=>Promise<boolean>) {
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState<string|null>(null);
  const busyRef=useRef(false);
  const alive=useRef(true);
  const request=useRef({digest:"",id:""});
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
  const save=async(value:unknown,operation:(id:string)=>Promise<TeamMutationResult>)=>{
    if(busyRef.current)return false;
    const digest=JSON.stringify(value);
    if(request.current.digest!==digest)request.current={digest,id:crypto.randomUUID()};
    busyRef.current=true;setBusy(true);setError(null);
    try {
      const result=await operation(request.current.id);
      if(!alive.current)return false;
      if(result.outcome!=="applied")setError(result.reason==="conflict"||result.reason==="busy"?result.reason:result.message??result.reason??"Save failed");
      return await onSaved(result);
    }catch(cause){if(alive.current)setError(cause instanceof Error?cause.message:String(cause));return false;}
    finally{busyRef.current=false;if(alive.current)setBusy(false);}
  };
  return {save,busy,error};
}

function TeamCheck({label,checked,disabled,onChange}:{label:string;checked:boolean;disabled:boolean;onChange:(value:boolean)=>void}) {
  return <label className="agent-preset-modes__option"><input type="checkbox" checked={checked} disabled={disabled} onChange={e=>onChange(e.target.checked)}/><span>{label}</span></label>;
}

function TeamAgentEditor({initial,memberID,revision,tabId,workspaceKey,onSaved,onCancel,onReload}:{initial:TeamAgentSettings;memberID:string;revision:number;tabId:string;workspaceKey:string;onSaved:(result:TeamMutationResult)=>Promise<boolean>;onCancel:()=>void;onReload:()=>void}) {
  const t=useT();
  const bt=useBotText();
  const [draft,setDraft]=useState(initial);
  const [presetID,setPresetID]=useState<string|undefined>();
  const [editingPreset,setEditingPreset]=useState(false);
  const [editingIcon,setEditingIcon]=useState(false);
  const [channelsOpen,setChannelsOpen]=useState(false);
  const [models,setModels]=useState<ModelInfo[]>([]);
  const [modelError,setModelError]=useState<string|null>(null);
  const presets=useAgentPresets(tabId,workspaceKey);
  const operation=useTeamSave(onSaved);
  const patch=(next:Partial<TeamAgentSettings>)=>setDraft(previous=>({...previous,...next}));
  const dirty=editingIcon||JSON.stringify(draft)!==JSON.stringify(initial)||presetID!==undefined;
  const invalid=!draft.name.trim()||draft.name.length>40||editingIcon;
  const submit=()=>operation.save(draft,requestId=>app.SaveTeamAgent({id:memberID,expectedRevision:revision,requestId,settings:draft}));
  const guard=useSettingsDraft({label:t("team.members"),order:1,dirty,busy:operation.busy,canSave:!invalid,save:submit,discard:()=>{setDraft(initial);setPresetID(undefined);}});
  useEffect(()=>{let current=true;void app.Models().then(list=>{if(current)setModels(list);},cause=>{if(current)setModelError(String(cause));});return()=>{current=false;};},[]);
  if(channelsOpen) return <><SettingsSubpageHeader title={`${draft.name} · ${bt.channelsSection}`} description={bt.memberHint} onBack={()=>setChannelsOpen(false)}/><BotChannelsSettingsPage memberId={memberID}/></>;
  if(editingPreset) {
    const p=draft.preset;
    const snapshot:AgentPresetDraft={id:p?.id&&p.id!=="default"?p.id:"team-member",name:p?.name||draft.name,description:p?.description??"",icon:p?.icon??"",iconImage:p?.iconImage??"",iconColor:p?.iconColor??"",body:p?.body??"",keepCoding:p?.keepCoding??true,complete:p?.complete??false,includeRuntimeContext:p?.includeRuntimeContext??true,tools:p?.restrictTools?p.tools:null,scope:"user"};
    return <AgentPresetEditor initial={snapshot} editing presets={presets.view?.presets??[]} busy={operation.busy} onCancel={()=>setEditingPreset(false)} onSave={async value=>{patch({preset:presetCopy(value)});setPresetID(undefined);setEditingPreset(false);return true;}}/>;
  }
  return <><form onSubmit={event=>{event.preventDefault();if(!invalid&&!operation.busy)void submit();}}>
    <SettingsSubpageHeader title={memberID?draft.name:t("team.newAgent")} description={t("team.presetHint")} onBack={onCancel}/>
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):operation.error}</SettingsFeedback>}
    {operation.error==="conflict"&&<button className="btn" type="button" onClick={onReload}>{t("team.reloadSaved")}</button>}
    {(presets.error||modelError)&&<SettingsFeedback kind="error">{presets.error||modelError}</SettingsFeedback>}
    <SettingsSection>
      <SettingsField label={t("team.name")}><input className="mem-input set-grow" autoFocus required maxLength={40} value={draft.name} disabled={operation.busy} onChange={e=>patch({name:e.target.value})}/></SettingsField>
      <SettingsField label={t("team.role")}><input className="mem-input set-grow" maxLength={24} value={draft.role} disabled={operation.busy} onChange={e=>patch({role:e.target.value})}/></SettingsField>
      <SettingsField label={t("agentPresets.icon")}><AgentPresetIconPicker icon={draft.avatar.glyph} color={draft.avatar.color} image={draft.avatar.image} disabled={operation.busy} onEditingChange={setEditingIcon} onChange={(glyph,image,color)=>patch({avatar:{glyph,image,color}})}/></SettingsField>
      <SettingsField label={t("team.instructions")} stacked><textarea className="mem-textarea" rows={6} maxLength={8000} value={draft.instructions} disabled={operation.busy} onChange={e=>patch({instructions:e.target.value})}/></SettingsField>
      <SettingsField label={t("team.preset")} hint={t("team.presetHint")}><SettingsSelect value={presetID??""} disabled={operation.busy} onValueChange={id=>{setPresetID(id||undefined);const p=presets.view?.presets.find(p=>p.id===id);if(p)patch({preset:presetCopy(p),avatar:{glyph:p.icon,color:p.iconColor,image:p.iconImage}});}} options={[{value:"",label:t("common.none")},...(presets.view?.presets??[]).map(p=>({value:p.id,label:p.name}))]}/></SettingsField>
      <button className="btn btn--small" type="button" disabled={operation.busy} onClick={()=>setEditingPreset(true)}>{t("agentPresets.edit")}</button>
      <SettingsField label={t("team.model")}><SettingsSelect value={draft.initialModel} disabled={operation.busy} onValueChange={initialModel=>patch({initialModel})} options={[{value:"",label:t("agentPresets.default")},...models.map(m=>({value:m.ref,label:m.displayName||m.ref}))]}/></SettingsField>
      <SettingsField label={t("team.folder")}><input className="mem-input set-grow" value={draft.folder} disabled={operation.busy} onChange={e=>patch({folder:e.target.value})}/></SettingsField>
      <SettingsField label={t("settings.tab.permissions")}><PermissionPresetChoice value={normalizeToolApprovalMode(draft.permission)} disabled={operation.busy} scopeKey={`team-member:${memberID}:${revision}`} projectConfirmationKey={fullAccessProjectConfirmationKey({workspacePath:draft.folder})} onPick={permission=>patch({permission})}/></SettingsField>
      <TeamCheck label={t("team.main")} checked={draft.main} disabled={operation.busy} onChange={main=>patch({main})}/>
      <TeamCheck label={t("team.pinned")} checked={draft.pinned} disabled={operation.busy} onChange={pinned=>patch({pinned})}/>
      <TeamCheck label={t("team.hidden")} checked={draft.hidden} disabled={operation.busy} onChange={hidden=>patch({hidden})}/>
    </SettingsSection>
    <div className="settings-save-bar"><span role="status">{dirty?t("settings.models.unsaved"):t("settings.saveExplicitly")}</span><button className="btn" type="button" disabled={operation.busy} onClick={()=>guard.leave(onCancel)}>{t("common.cancel")}</button><button className="btn btn--primary" type="submit" disabled={invalid||operation.busy||!dirty}>{t("common.save")}</button></div>
  </form>
  {memberID ? <SettingsSection title={bt.channelsSection} description={bt.memberHint}><button className="btn" type="button" disabled={dirty||operation.busy} onClick={()=>setChannelsOpen(true)}>{bt.channelsSection}</button>{dirty&&<SettingsFeedback kind="info">{bt.memberSavedFirst}</SettingsFeedback>}</SettingsSection> : <SettingsFeedback kind="info">{bt.memberSavedFirst}</SettingsFeedback>}
  </>;
}

function TeamGroupEditor({initial,groupID,revision,members,onSaved,onCancel,onReload}:{initial:TeamGroupSettings;groupID:string;revision:number;members:Agent[];onSaved:(result:TeamMutationResult)=>Promise<boolean>;onCancel:()=>void;onReload:()=>void}) {
  const t=useT();
  const [draft,setDraft]=useState(initial);
  const operation=useTeamSave(onSaved);
  const patch=(next:Partial<TeamGroupSettings>)=>setDraft(previous=>({...previous,...next}));
  const dirty=JSON.stringify(draft)!==JSON.stringify(initial);
  const invalid=!draft.name.trim()||draft.members.length<2||draft.members.length>12||draft.mode==="admin"&&!draft.members.includes(draft.admin);
  const submit=()=>operation.save(draft,requestId=>app.SaveTeamGroup({id:groupID,expectedRevision:revision,requestId,settings:draft}));
  const guard=useSettingsDraft({label:t("team.groups"),dirty,busy:operation.busy,canSave:!invalid,save:submit,discard:()=>setDraft(initial)});
  return <form onSubmit={event=>{event.preventDefault();if(!invalid&&!operation.busy)void submit();}}>
    <SettingsSubpageHeader title={groupID?draft.name:t("team.newGroup")} onBack={onCancel}/>
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):operation.error==="busy"?t("team.busy"):operation.error}</SettingsFeedback>}
    {operation.error==="conflict"&&<button className="btn" type="button" onClick={onReload}>{t("team.reloadSaved")}</button>}
    {operation.error==="busy"&&<button className="btn" type="button" onClick={()=>void app.StopTeamGroup(groupID).then(()=>notifyTeamChanged()).catch(()=>{})}>{t("team.stop")}</button>}
    <SettingsSection>
      <SettingsField label={t("team.name")}><input className="mem-input set-grow" autoFocus required maxLength={60} value={draft.name} disabled={operation.busy} onChange={e=>patch({name:e.target.value})}/></SettingsField>
      <SettingsField label={t("team.notice")} stacked><textarea className="mem-textarea" rows={4} maxLength={2000} value={draft.notice} disabled={operation.busy} onChange={e=>patch({notice:e.target.value})}/></SettingsField>
      <SettingsField label={t("team.members")} stacked><div>{members.map(member=><TeamCheck key={member.id} label={member.name} checked={draft.members.includes(member.id)} disabled={operation.busy} onChange={checked=>patch({members:checked?[...draft.members,member.id]:draft.members.filter(id=>id!==member.id)})}/>)}</div></SettingsField>
      <SettingsField label={t("team.admin")}><SettingsSelect value={draft.admin} disabled={operation.busy} onValueChange={admin=>patch({admin})} options={[{value:"",label:t("common.none")},...members.filter(member=>draft.members.includes(member.id)).map(member=>({value:member.id,label:member.name}))]}/></SettingsField>
      <SettingsField label={t("team.mode")}><SettingsSelect value={draft.mode} disabled={operation.busy} onValueChange={mode=>patch({mode})} options={[{value:"everyone",label:t("team.modeEveryone")},{value:"admin",label:t("team.modeAdmin")} ]}/></SettingsField>
      <SettingsField label={t("team.relayLimit")}><input className="mem-input" type="number" min={0} max={200} value={draft.relayLimit} disabled={operation.busy} onChange={e=>patch({relayLimit:Number(e.target.value)})}/></SettingsField>
      <SettingsField label={t("team.folder")}><input className="mem-input set-grow" value={draft.folder} disabled={operation.busy} onChange={e=>patch({folder:e.target.value})}/></SettingsField>
      <TeamCheck label={t("team.pinned")} checked={draft.pinned} disabled={operation.busy} onChange={pinned=>patch({pinned})}/>
      <TeamCheck label={t("team.hidden")} checked={draft.hidden} disabled={operation.busy} onChange={hidden=>patch({hidden})}/>
      <TeamCheck label={t("team.archived")} checked={draft.archived} disabled={operation.busy} onChange={archived=>patch({archived})}/>
    </SettingsSection>
    <div className="settings-save-bar"><span role="status">{dirty?t("settings.models.unsaved"):t("settings.saveExplicitly")}</span><button className="btn" type="button" disabled={operation.busy} onClick={()=>guard.leave(onCancel)}>{t("common.cancel")}</button><button className="btn btn--primary" type="submit" disabled={invalid||operation.busy||!dirty}>{t("common.save")}</button></div>
  </form>;
}
