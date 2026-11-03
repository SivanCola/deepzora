import { useEffect, useState } from "react";
import type { MemoryFact, TeamMemoryTarget, TeamMemoryView, TeamView, TeamMemoryReviewStatus } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { notifyTeamChanged } from "../lib/useTeam";
import { useTeamOperation } from "../lib/useTeamOperation";
import { SettingsFeedback, useSettingsDraft, useSettingsExperience } from "./SettingsExperience";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";
import { InlineConfirmButton } from "./InlineConfirmButton";

export function TeamMemorySettings({team,reload}:{team:TeamView;reload:()=>Promise<unknown>}){
  const t=useT(),{requestLeave}=useSettingsExperience();
  const [scope,setScope]=useState("team"),[version,setVersion]=useState(0);
  const [view,setView]=useState<TeamMemoryView|null>(null),[error,setError]=useState("");
  const [editing,setEditing]=useState<MemoryFact|null>(null);
  const targets=[{value:"team",label:t("team.memoryTeam"),target:{tier:"team"}},...team.roster.agents.map(a=>({value:`agent:${a.id}`,label:`${t("team.memoryPrivate")} · ${a.name}`,target:{tier:"private",agentId:a.id}})),...team.roster.groups.map(g=>({value:`group:${g.id}`,label:`${t("team.groups")} · ${g.name}`,target:{tier:"group",groupId:g.id}}))];
  const target:TeamMemoryTarget|undefined=targets.find(x=>x.value===scope)?.target;
  const targetKey=JSON.stringify(target??null);
  useEffect(()=>{let active=true;setView(null);setError("");const selected=JSON.parse(targetKey) as TeamMemoryTarget|null;if(selected)void app.GetTeamMemory(selected).then(value=>{if(active)setView(value);},cause=>{if(active)setError(String(cause));});return()=>{active=false;};},[targetKey,version]);
  const blank:MemoryFact={name:"",description:"",body:"",type:"user",scope:"project",freshness:""};
  const changed=()=>{setEditing(null);setVersion(v=>v+1);notifyTeamChanged();};
  return <>
    <TeamReviewPreferences team={team} reload={reload}/>
    <SettingsSection title={t("team.memory")} description={t("team.memoryHint")}>
      <SettingsField label={t("team.memoryScope")}><SettingsSelect value={scope} options={targets} onValueChange={value=>requestLeave(()=>{setEditing(null);setScope(value);})}/></SettingsField>
      {error&&<SettingsFeedback kind="error">{error}</SettingsFeedback>}
      {!target&&<SettingsFeedback kind="error">{t("team.memoryTargetGone")}</SettingsFeedback>}
      {view&&target&&(editing?<TeamMemoryEditor key={`${scope}:${editing.id??"new"}`} target={target} view={view} initial={editing} writable={team.writable} onSaved={changed} onCancel={()=>setEditing(null)} onReload={()=>requestLeave(()=>{setEditing(null);setVersion(v=>v+1);})}/>:<>
        <button type="button" className="btn btn--small" disabled={!team.writable} onClick={()=>setEditing(blank)}>{t("team.memoryAdd")}</button>
        {view.facts.length===0&&<SettingsFeedback kind="empty">{t("common.none")}</SettingsFeedback>}
        {view.facts.map(fact=><article className="cap-skill-card" key={fact.id}><h3>{fact.description||fact.name}</h3><p className="cap-skill-card__desc">{fact.body}</p><button type="button" className="btn btn--small" disabled={!team.writable} onClick={()=>setEditing(fact)}>{t("common.edit")}</button></article>)}
      </>)}
    </SettingsSection>
  </>;
}

function TeamMemoryEditor({target,view,initial,writable,onSaved,onCancel,onReload}:{target:TeamMemoryTarget;view:TeamMemoryView;initial:MemoryFact;writable:boolean;onSaved:()=>void;onCancel:()=>void;onReload:()=>void}){
  const t=useT(),operation=useTeamOperation();
  const [draft,setDraft]=useState(initial);
  const valid=Boolean(draft.description.trim()&&draft.body.trim());
  const commit=(archive:boolean)=>operation.run({target,draft,archive,revision:view.rosterRevision},requestId=>app.SaveTeamMemory({target,expectedRosterRevision:view.rosterRevision,requestId,fact:draft,archive}),onSaved);
  const guard=useSettingsDraft({label:t("team.memory"),order:3,dirty:JSON.stringify(draft)!==JSON.stringify(initial),busy:operation.busy,canSave:writable&&valid,save:()=>commit(false),discard:()=>setDraft(initial)});
  return <form onSubmit={event=>{event.preventDefault();if(valid)void commit(false);}} aria-busy={operation.busy}>
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):operation.error}</SettingsFeedback>}
    {operation.error==="conflict"&&<button className="btn" type="button" onClick={onReload}>{t("team.reloadSaved")}</button>}
    <SettingsField label={t("team.memoryDescription")}><input className="mem-input set-grow" value={draft.description} disabled={operation.busy} maxLength={400} onChange={e=>setDraft({...draft,description:e.target.value,name:draft.id?draft.name:e.target.value})}/></SettingsField>
    <SettingsField label={t("team.memoryBody")} stacked><textarea className="mem-textarea" rows={5} value={draft.body} disabled={operation.busy} maxLength={16000} onChange={e=>setDraft({...draft,body:e.target.value})}/></SettingsField>
    <div className="agent-preset-row__actions"><button className="btn btn--primary" disabled={!writable||!valid||operation.busy}>{t("common.save")}</button><button className="btn" type="button" disabled={operation.busy} onClick={()=>guard.leave(onCancel)}>{t("common.cancel")}</button>
      {draft.id&&<InlineConfirmButton label={t("team.memoryArchive")} confirmLabel={t("team.memoryArchiveConfirm")} cancelLabel={t("common.cancel")} disabled={!writable||operation.busy} onConfirm={async()=>{await commit(true);}}/>}
    </div>
  </form>;
}

function TeamReviewPreferences({team,reload}:{team:TeamView;reload:()=>Promise<unknown>}){
  const t=useT(),operation=useTeamOperation();
  const initial={enabled:team.roster.prefs.memoryReview,model:team.roster.prefs.memoryReviewModel??""};
  const [draft,setDraft]=useState(initial),[saved,setSaved]=useState(initial),[revision,setRevision]=useState(team.roster.revision);
  const [status,setStatus]=useState<TeamMemoryReviewStatus[]>([]),[error,setError]=useState("");
  useEffect(()=>{let active=true;void app.GetTeamMemoryReviewStatus().then(list=>{if(active)setStatus(list);},cause=>{if(active)setError(String(cause));});return()=>{active=false;};},[team]);
  const dirty=JSON.stringify(draft)!==JSON.stringify(saved);
  useEffect(()=>{if(!dirty&&!operation.busy){const value={enabled:team.roster.prefs.memoryReview,model:team.roster.prefs.memoryReviewModel??""};setDraft(value);setSaved(value);setRevision(team.roster.revision);}},[team.roster.revision,team.roster.prefs,dirty,operation.busy]);
  const save=()=>operation.run({draft,revision},requestId=>app.SaveTeamMemoryPreferences({expectedRevision:revision,requestId,...draft}),async()=>{setSaved(draft);await reload();notifyTeamChanged();});
  useSettingsDraft({label:t("team.memoryReview"),order:2,dirty,busy:operation.busy,canSave:team.writable,save,discard:()=>setDraft(saved)});
  return <SettingsSection title={t("team.memoryReview")} description={t("team.memoryReviewHint")}>
    {(operation.error||error)&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):operation.error||error}</SettingsFeedback>}
    <SettingsField label={t("team.memoryReview")}><input type="checkbox" checked={draft.enabled} disabled={!team.writable||operation.busy} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/></SettingsField>
    <SettingsField label={t("team.memoryReviewModel")} hint={t("team.memoryReviewModelHint")}><input className="mem-input set-grow" value={draft.model} disabled={!team.writable||operation.busy} onChange={e=>setDraft({...draft,model:e.target.value})}/></SettingsField>
    <button className="btn" type="button" disabled={!team.writable||!dirty||operation.busy} onClick={()=>void save()}>{t("common.save")}</button>
    {status.slice(-5).reverse().map((item,index)=><p role="status" key={`${item.at}:${index}`}>{item.agentId} · {new Date(item.at).toLocaleString()} · {item.outcome==="failed"?`${t("team.memoryReviewFailed")}: ${item.error??""}`:`+${item.added} / ~${item.edited} / −${item.archived}`}</p>)}
  </SettingsSection>;
}
