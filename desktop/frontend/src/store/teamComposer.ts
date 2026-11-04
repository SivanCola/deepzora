import { useEffect } from "react";
import { create } from "zustand";
import { app } from "../lib/bridge";
import type { TeamGroupDraft, TeamGroupDraftView } from "../generated/desktopContract.generated";

export type TeamComposerDraft=TeamGroupDraft;
export const emptyTeamDraft:TeamComposerDraft={requestId:"",text:"",mentions:[],everyone:false};
type Entry={revision?:number;version:number;saved:number;tasks:Set<Promise<unknown>>;loading?:Promise<void>;saving?:Promise<void>;timer?:ReturnType<typeof setTimeout>;error?:string};
const entries=new Map<string,Entry>();
let exiting=false,exitGeneration=0;
export const useTeamComposerStore=create<{drafts:Record<string,TeamComposerDraft>;serial:number;set:(group:string,draft:TeamComposerDraft)=>void}>((set)=>({drafts:{},serial:0,set:(group,draft)=>{
  const entry=entryFor(group);if(exiting||entry.revision===undefined||entry.error)return;
  entry.version++;
  set(state=>({drafts:{...state.drafts,[group]:{...draft,requestId:crypto.randomUUID()}}}));
  if(entry.timer)clearTimeout(entry.timer);
  entry.timer=setTimeout(()=>{void flushTeamDraft(group).catch(()=>{});},250);
}}));
const publish=()=>useTeamComposerStore.setState(state=>({serial:state.serial+1}));
function entryFor(group:string){let entry=entries.get(group);if(!entry){entry={version:0,saved:0,tasks:new Set()};entries.set(group,entry);}return entry;}
export function trackTeamSubmission<T>(group:string,run:()=>Promise<T>):Promise<T>{
  const entry=entryFor(group),task=Promise.resolve().then(run);entry.tasks.add(task);
  void task.finally(()=>entry.tasks.delete(task)).catch(()=>{});return task;
}
export function clearSubmittedTeamDraft(group:string,submitted:TeamComposerDraft){
  if(useTeamComposerStore.getState().drafts[group]!==submitted)return;
  entryFor(group).version++;
  useTeamComposerStore.setState(state=>({drafts:{...state.drafts,[group]:emptyTeamDraft}}));
}
function install(group:string,view:TeamGroupDraftView){
  const entry=entryFor(group);entry.revision=view.revision;entry.saved=entry.version;entry.error=undefined;
  useTeamComposerStore.setState(state=>({drafts:{...state.drafts,[group]:view.draft}}));
}
export async function loadTeamDraft(group:string,discard=false):Promise<void>{
  const entry=entryFor(group);
  if(entry.loading)return entry.loading;
  if(entry.revision!==undefined&&!discard)return;
  entry.loading=(async()=>{
    try{install(group,await app.ReadTeamGroupDraft(group));}
    catch(cause){entry.error=String(cause);throw cause;}
    finally{entry.loading=undefined;publish();}
  })();publish();return entry.loading;
}
export async function flushTeamDraft(group:string):Promise<void>{
  const entry=entryFor(group);
  if(entry.loading)await entry.loading;
  if(entry.saving){await entry.saving;if(entry.saved<entry.version)return flushTeamDraft(group);return;}
  if(entry.saved===entry.version)return;
  if(entry.revision===undefined||entry.error)throw new Error(entry.error||"Group input is not loaded");
  entry.saving=(async()=>{
    while(entry.saved<entry.version){
      const version=entry.version,draft=useTeamComposerStore.getState().drafts[group]??emptyTeamDraft;
      const result=await app.SaveTeamGroupDraft({groupId:group,expectedRevision:entry.revision!,draft});
      if(result.outcome!=="applied")throw new Error(result.reason??"Group input could not be saved");
      entry.revision=result.view.revision;entry.saved=version;
    }
  })();
  try{await entry.saving;}catch(cause){entry.error=String(cause);throw cause;}finally{entry.saving=undefined;publish();}
}
export async function flushAllTeamDrafts(){
  const owner=++exitGeneration;
  exiting=true;
  try{for(const [group,entry] of entries){while(entry.tasks.size)await Promise.allSettled([...entry.tasks]);if(owner!==exitGeneration)throw new Error("Draft save was superseded");await flushTeamDraft(group);}}
  catch(cause){if(owner===exitGeneration)exiting=false;throw cause;}
}
export function resumeTeamDraftEditing(){exitGeneration++;exiting=false;publish();}
export function useTeamDraft(group:string){
  useTeamComposerStore(state=>state.serial);
  const draft=useTeamComposerStore(state=>state.drafts[group]??emptyTeamDraft),entry=entryFor(group);
  useEffect(()=>{void loadTeamDraft(group).catch(()=>{});return()=>{if(entry.version>entry.saved)void flushTeamDraft(group).catch(()=>{});};},[group,entry]);
  return {draft,ready:entry.revision!==undefined&&!entry.loading&&!exiting,error:entry.error,
    set:(value:TeamComposerDraft)=>useTeamComposerStore.getState().set(group,value),
    retry:async()=>{if(entry.error?.includes("conflict"))return;entry.error=undefined;if(entry.revision===undefined)await loadTeamDraft(group);else await flushTeamDraft(group);},
    reload:()=>loadTeamDraft(group,true)};
}
