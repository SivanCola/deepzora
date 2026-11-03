import type { MemoryFact, TeamScheduleView, TeamView } from "../generated/desktopContract.generated";
import type { TeamHostBindings } from "./teamBindings";

type KnowledgeBindings=Pick<TeamHostBindings,"GetTeamMemory"|"SaveTeamMemory"|"GetTeamMemoryReviewStatus"|"SaveTeamMemoryPreferences"|"GetTeamSchedules"|"SaveTeamSchedule"|"RunTeamSchedule">;

// Browser interaction fixture only. It records acceptance and never fabricates
// a model completion, memory review request, or scheduler background execution.
export function makeTeamKnowledgeMock(team:TeamView):KnowledgeBindings{
  const facts=new Map<string,MemoryFact[]>(),receipts=new Map<string,{digest:string;result:unknown}>();
  const schedules:TeamScheduleView={revision:0,schedules:[]};
  const snapshot=()=>structuredClone(schedules);
  const retry=<T>(key:string,input:unknown):T|undefined=>{
    const receipt=receipts.get(key);
    if(!receipt)return;
    if(receipt.digest!==JSON.stringify(input))throw new Error("request_reused");
    return structuredClone(receipt.result) as T;
  };
  const keep=<T>(key:string,input:unknown,result:T):T=>{receipts.set(key,{digest:JSON.stringify(input),result:structuredClone(result)});return result;};
  return {
    GetTeamMemory:async target=>({rosterRevision:team.roster.revision,facts:structuredClone(facts.get(JSON.stringify(target))??[])}),
    SaveTeamMemory:async req=>{
      const old=retry<Awaited<ReturnType<KnowledgeBindings["SaveTeamMemory"]>>>(req.requestId,req);if(old)return old;
      if(req.expectedRosterRevision!==team.roster.revision)return {outcome:"rejected",reason:"conflict",fact:req.fact};
      const key=JSON.stringify(req.target),list=facts.get(key)??[];
      const index=list.findIndex(f=>f.id===req.fact.id);
      if(req.fact.id&&(index<0||list[index].revision!==req.fact.revision))return {outcome:"rejected",reason:"conflict",fact:req.fact};
      const fact={...structuredClone(req.fact),id:req.fact.id||crypto.randomUUID(),revision:(req.fact.revision??0)+1};
      if(req.archive)list.splice(index,1);else if(index>=0)list[index]=fact;else list.push(fact);
      facts.set(key,list);return keep(req.requestId,req,{outcome:"applied",fact});
    },
    GetTeamMemoryReviewStatus:async()=>[],
    SaveTeamMemoryPreferences:async req=>{
      const old=retry<Awaited<ReturnType<KnowledgeBindings["SaveTeamMemoryPreferences"]>>>(req.requestId,req);if(old)return old;
      if(req.expectedRevision!==team.roster.revision)return {outcome:"rejected",reason:"conflict",view:structuredClone(team)};
      team.roster.prefs={memoryReview:req.enabled,memoryReviewModel:req.model,memoryReviewStartedAt:new Date().toISOString()};team.roster.revision++;
      return keep(req.requestId,req,{outcome:"applied",view:structuredClone(team)});
    },
    GetTeamSchedules:async()=>snapshot(),
    SaveTeamSchedule:async req=>{
      const old=retry<Awaited<ReturnType<KnowledgeBindings["SaveTeamSchedule"]>>>(req.requestId,req);if(old)return old;
      if(req.expectedRevision!==schedules.revision||req.expectedRosterRevision!==team.roster.revision)return {outcome:"rejected",reason:"conflict",view:snapshot()};
      const sc=structuredClone(req.schedule),index=schedules.schedules.findIndex(s=>s.id===sc.id);
      if(sc.id&&index<0)return {outcome:"rejected",reason:"target_invalid",view:snapshot()};
      if(req.delete)schedules.schedules.splice(index,1);else if(index>=0)schedules.schedules[index]=sc;else{sc.id=crypto.randomUUID();schedules.schedules.push(sc);}
      schedules.revision++;return keep(req.requestId,req,{outcome:"applied",targetId:sc.id,view:snapshot()});
    },
    RunTeamSchedule:async req=>{
      const old=retry<Awaited<ReturnType<KnowledgeBindings["RunTeamSchedule"]>>>(req.requestId,req);if(old)return old;
      if(req.expectedRevision!==schedules.revision)return {outcome:"rejected",reason:"conflict",view:snapshot()};
      return keep(req.requestId,req,{outcome:"accepted",targetId:req.id,view:snapshot()});
    },
  };
}
