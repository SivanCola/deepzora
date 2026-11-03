import { useCallback, useEffect, useRef, useState } from "react";
import type { Schedule, TeamScheduleView, TeamView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { notifyTeamChanged } from "../lib/useTeam";
import { useTeamOperation } from "../lib/useTeamOperation";
import { InlineConfirmButton } from "./InlineConfirmButton";
import { SettingsFeedback, useSettingsDraft } from "./SettingsExperience";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";

export function TeamScheduleSettings({team}:{team:TeamView}){
  const t=useT(),operation=useTeamOperation();
  const [view,setView]=useState<TeamScheduleView|null>(null),[error,setError]=useState("");
  const [editing,setEditing]=useState<{schedule:Schedule;revision:number;rosterRevision:number}|null>(null),[notice,setNotice]=useState("");
  const sequence=useRef(0);
  const reload=useCallback(async()=>{const request=++sequence.current;try{const result=await app.GetTeamSchedules();if(sequence.current===request){setView(result);setError("");}}catch(cause){if(sequence.current===request)setError(String(cause));}},[]);
  useEffect(()=>{void reload();return()=>{sequence.current++;};},[reload,team]);
  const begin=(schedule:Schedule)=>{if(view)setEditing({schedule,revision:view.revision,rosterRevision:team.roster.revision});};
  const blank=():Schedule=>({id:"",name:"",agent:team.roster.agents[0]?.id??"",prompt:"",repeat:"once",timezone:Intl.DateTimeFormat().resolvedOptions().timeZone,days:[],anchor:new Date(Date.now()+3600000).toISOString(),enabled:true,next:"0001-01-01T00:00:00Z",lastAt:"0001-01-01T00:00:00Z",intervalMinutes:60,clock:"09:00"});
  return <SettingsSection title={t("team.schedules")} description={t("team.scheduleHint")}>
    {(error||operation.error)&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):error||operation.error}</SettingsFeedback>}
    {notice&&<SettingsFeedback kind="success">{notice}</SettingsFeedback>}
    {editing?<TeamScheduleEditor key={`${editing.schedule.id}:${editing.revision}`} initial={editing.schedule} revision={editing.revision} rosterRevision={editing.rosterRevision} team={team} onSaved={async()=>{setEditing(null);await reload();notifyTeamChanged();}} onCancel={()=>setEditing(null)}/>:<>
      <button className="btn btn--small" type="button" disabled={!team.writable||!team.roster.agents.length||!view} onClick={()=>begin(blank())}>{t("team.scheduleNew")}</button>
      {view?.schedules.map(sc=><article className="cap-skill-card" key={sc.id}><h3>{sc.name}</h3><p className="cap-skill-card__desc">{sc.prompt}</p><p>{team.roster.agents.find(a=>a.id===sc.agent)?.name??team.roster.groups.find(g=>g.id===sc.group)?.name??t("team.memoryTargetGone")} · {sc.timezone}{sc.enabled&&sc.next&&sc.next!=="0001-01-01T00:00:00Z"?` · ${new Date(sc.next).toLocaleString()}`:""}</p><p role="status">{sc.lastOutcome??""}</p>
        <div className="agent-preset-row__actions"><button className="btn btn--small" type="button" disabled={!team.writable||operation.busy} onClick={()=>begin(sc)}>{t("common.edit")}</button>
          <button className="btn btn--small" type="button" disabled={!team.writable||operation.busy} onClick={()=>void operation.run({id:sc.id,revision:view.revision},requestId=>app.RunTeamSchedule({id:sc.id,expectedRevision:view.revision,requestId}),async()=>{setNotice(t("team.scheduleAccepted"));await reload();})}>{t("team.scheduleRun")}</button>
          <InlineConfirmButton label={t("common.delete")} confirmLabel={t("common.delete")} cancelLabel={t("common.cancel")} danger disabled={!team.writable||operation.busy} onConfirm={async()=>{await operation.run({sc,revision:view.revision,rosterRevision:team.roster.revision,delete:true},requestId=>app.SaveTeamSchedule({schedule:sc,expectedRevision:view.revision,expectedRosterRevision:team.roster.revision,requestId,delete:true}),async()=>{await reload();notifyTeamChanged();});}}/>
        </div>
      </article>)}
    </>}
  </SettingsSection>;
}

function TeamScheduleEditor({initial,revision,rosterRevision,team,onSaved,onCancel}:{initial:Schedule;revision:number;rosterRevision:number;team:TeamView;onSaved:()=>Promise<void>;onCancel:()=>void}){
  const t=useT(),operation=useTeamOperation(),[draft,setDraft]=useState(initial);
  const patch=(value:Partial<Schedule>)=>setDraft(old=>({...old,...value}));
  const target=draft.group?`group:${draft.group}`:`agent:${draft.agent??""}`;
  const targets=[...team.roster.agents.map(a=>({value:`agent:${a.id}`,label:a.name})),...team.roster.groups.filter(g=>!g.archived).map(g=>({value:`group:${g.id}`,label:g.name||g.id}))];
  const valid=Boolean(draft.name.trim()&&draft.prompt.trim()&&targets.some(item=>item.value===target)&&draft.timezone.trim()&&Number.isFinite(Date.parse(draft.anchor))&&(draft.repeat!=="every"||((draft.intervalMinutes??0)>=10&&(draft.intervalMinutes??0)<=10080))&&(draft.repeat!=="weekly"||draft.days.length>0));
  const save=()=>operation.run({draft,revision,rosterRevision},requestId=>app.SaveTeamSchedule({schedule:draft,expectedRevision:revision,expectedRosterRevision:rosterRevision,requestId,delete:false}),onSaved);
  const dirty=JSON.stringify(draft)!==JSON.stringify(initial);
  const guard=useSettingsDraft({label:t("team.schedules"),order:4,dirty,busy:operation.busy,canSave:valid&&team.writable,save,discard:()=>setDraft(initial)});
  return <form aria-busy={operation.busy} onSubmit={event=>{event.preventDefault();if(valid)void save();}}>
    {operation.error&&<SettingsFeedback kind="error">{operation.error==="conflict"?t("team.conflict"):operation.error}</SettingsFeedback>}
    <SettingsField label={t("team.name")}><input className="mem-input set-grow" value={draft.name} maxLength={60} disabled={operation.busy} onChange={e=>patch({name:e.target.value})}/></SettingsField>
    <SettingsField label={t("team.scheduleTarget")}><SettingsSelect value={target} options={targets} disabled={operation.busy} onValueChange={value=>{const [kind,id]=value.split(":");patch({agent:kind==="agent"?id:"",group:kind==="group"?id:""});}}/></SettingsField>
    <SettingsField label={t("team.schedulePrompt")} stacked><textarea className="mem-textarea" rows={4} value={draft.prompt} maxLength={2000} disabled={operation.busy} onChange={e=>patch({prompt:e.target.value})}/></SettingsField>
    <SettingsField label={t("team.scheduleRepeat")}><SettingsSelect value={draft.repeat} options={[{value:"once",label:t("team.scheduleOnce")},{value:"daily",label:t("team.scheduleDaily")},{value:"weekly",label:t("team.scheduleWeekly")},{value:"every",label:t("team.scheduleEvery")}]} disabled={operation.busy} onValueChange={repeat=>patch({repeat})}/></SettingsField>
    <SettingsField label={t("team.scheduleTimezone")}><input className="mem-input set-grow" value={draft.timezone} disabled={operation.busy} onChange={e=>patch({timezone:e.target.value})}/></SettingsField>
    {(draft.repeat==="once"||draft.repeat==="every")&&<SettingsField label={t("team.scheduleAnchor")}><input className="mem-input set-grow" value={draft.anchor} placeholder="2026-11-04T09:00:00+08:00" disabled={operation.busy} onChange={e=>patch({anchor:e.target.value})}/></SettingsField>}
    {(draft.repeat==="daily"||draft.repeat==="weekly")&&<SettingsField label={t("team.scheduleClock")}><input className="mem-input" type="time" value={draft.clock??"09:00"} disabled={operation.busy} onChange={e=>patch({clock:e.target.value})}/></SettingsField>}
    {draft.repeat==="every"&&<SettingsField label={t("team.scheduleInterval")}><input className="mem-input" type="number" min={10} max={10080} value={draft.intervalMinutes??60} disabled={operation.busy} onChange={e=>patch({intervalMinutes:Number(e.target.value)})}/></SettingsField>}
    {draft.repeat==="weekly"&&<SettingsField label={t("team.scheduleDays")}><div className="agent-preset-row__actions">{[0,1,2,3,4,5,6].map(day=><label key={day}><input type="checkbox" checked={draft.days.includes(day)} disabled={operation.busy} onChange={e=>patch({days:e.target.checked?[...draft.days,day]:draft.days.filter(d=>d!==day)})}/>{new Intl.DateTimeFormat(undefined,{weekday:"short"}).format(new Date(2023,0,1+day))}</label>)}</div></SettingsField>}
    <SettingsField label={t("team.scheduleEnabled")}><input type="checkbox" checked={draft.enabled} disabled={operation.busy} onChange={e=>patch({enabled:e.target.checked})}/></SettingsField>
    <div className="settings-save-bar"><button className="btn" type="button" disabled={operation.busy} onClick={()=>guard.leave(onCancel)}>{t("common.cancel")}</button><button className="btn btn--primary" disabled={!valid||!dirty||!team.writable||operation.busy}>{t("common.save")}</button></div>
  </form>;
}
