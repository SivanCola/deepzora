import { useEffect, useRef, useState } from "react";
import type { TeamUsage, TeamView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { formatMoneyAmount } from "../lib/money";
import { SettingsFeedback } from "./SettingsExperience";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";

function localDay(date:Date){return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")}`;}
export function TeamUsageSettings({team}:{team:TeamView}){
  const t=useT();
  const [from,setFrom]=useState(()=>localDay(new Date(Date.now()-29*86400000))),[to,setTo]=useState(()=>localDay(new Date()));
  const [agent,setAgent]=useState("all"),[version,setVersion]=useState(0);
  const [rows,setRows]=useState<TeamUsage[]>([]),[error,setError]=useState(""),[busy,setBusy]=useState(false);
  const generation=useRef(0);
  useEffect(()=>{const own=++generation.current;setBusy(true);void app.GetTeamUsage({from,to,agentId:""}).then(list=>{if(own===generation.current){setRows(list);setError("");}},cause=>{if(own===generation.current)setError(String(cause));}).finally(()=>{if(own===generation.current)setBusy(false);});return()=>{generation.current++;};},[from,to,version]);
  const members=[...team.roster.agents,...team.roster.retiredAgents];
  const visible=rows.filter(row=>agent==="all"||(agent==="unattributed"?!row.agentId:row.agentId===agent));
  const options=[{value:"all",label:t("team.usageAll")},{value:"unattributed",label:t("team.usageUnattributed")},...members.map(member=>({value:member.id,label:member.name}))];
  return <SettingsSection title={t("team.usage")} description={t("team.usageHint")}>
    <SettingsField label={t("team.usageFrom")}><input className="mem-input" type="date" value={from} onChange={e=>setFrom(e.target.value)}/></SettingsField>
    <SettingsField label={t("team.usageTo")}><input className="mem-input" type="date" value={to} onChange={e=>setTo(e.target.value)}/></SettingsField>
    <SettingsField label={t("team.members")}><SettingsSelect value={agent} options={options} onValueChange={setAgent}/></SettingsField>
    <button className="btn btn--small" type="button" disabled={busy} onClick={()=>setVersion(v=>v+1)}>{t("summary.refresh")}</button>
    {error&&<SettingsFeedback kind="error">{error}</SettingsFeedback>}
    {busy&&<SettingsFeedback kind="loading"/>}
    <p role="status">{t("team.usageTotals",{tokens:visible.reduce((sum,row)=>sum+row.tokens,0).toLocaleString(),requests:visible.reduce((sum,row)=>sum+row.requests,0).toLocaleString()})}</p>
    {visible.map((row,index)=>{
      const money=row.cost.originalTotals?.length?row.cost.originalTotals:[row.cost.original];
      const cost=money.filter(item=>Boolean(item.currency)).map(item=>formatMoneyAmount(item.amount,item.currency)).join(" + ");
      return <article className="cap-skill-card" key={`${row.agentId}:${row.seatId}:${row.model}:${row.usageSource}:${index}`}><h3>{members.find(member=>member.id===row.agentId)?.name||row.agentId||t("team.usageUnattributed")}</h3><p>{row.model} · {row.groupId?team.roster.groups.find(group=>group.id===row.groupId)?.name??row.groupId:t("team.memoryPrivate")}</p><p>{row.seatId||t("team.usageUnattributed")}{row.usageSource?` · ${row.usageSource}`:""}</p><p>{t("team.usageTotals",{tokens:row.tokens.toLocaleString(),requests:row.requests.toLocaleString()})}</p><p>{row.cost.estimated?"≈ ":""}{cost||t("team.usageUnpriced")}{!row.cost.costComplete?` · ${t("team.usageIncomplete")}`:""}</p></article>;
    })}
  </SettingsSection>;
}
