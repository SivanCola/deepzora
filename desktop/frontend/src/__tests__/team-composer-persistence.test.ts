import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import { installDesktopHostStub } from "./desktopHostStub";
import type { SaveTeamGroupDraftRequest, TeamGroupDraftView } from "../generated/desktopContract.generated";
import { clearSubmittedTeamDraft, emptyTeamDraft, flushAllTeamDrafts, flushTeamDraft, loadTeamDraft, resumeTeamDraftEditing, trackTeamSubmission, useTeamComposerStore } from "../store/teamComposer";

const dom=new JSDOM("<div></div>",{url:"http://localhost"});
Object.assign(globalThis,{window:dom.window,document:dom.window.document});
const saved=new Map<string,TeamGroupDraftView>();
let pause:Promise<void>|undefined;
let entered:(()=>void)|undefined;
let loseResponse=false;
const requests:SaveTeamGroupDraftRequest[]=[];
installDesktopHostStub({
  ReadTeamGroupDraft:async (group:string)=>structuredClone(saved.get(group)??{revision:0,draft:emptyTeamDraft}),
  SaveTeamGroupDraft:async (request:SaveTeamGroupDraftRequest)=>{
    requests.push(structuredClone(request));
    entered?.();if(pause)await pause;
    const previous=saved.get(request.groupId);
    if(previous?.revision!==undefined&&previous.revision!==request.expectedRevision){
      if(previous.revision===request.expectedRevision+1&&JSON.stringify(previous.draft)===JSON.stringify(request.draft))return {outcome:"applied",view:structuredClone(previous)};
      return {outcome:"rejected",reason:"conflict",view:structuredClone(previous)};
    }
    const next={revision:request.expectedRevision+1,draft:structuredClone(request.draft)};saved.set(request.groupId,next);
    if(loseResponse){loseResponse=false;throw new Error("lost response");}
    return {outcome:"applied",view:structuredClone(next)};
  },
});
const edit=(group:string,text:string)=>useTeamComposerStore.getState().set(group,{...emptyTeamDraft,text});
try {
  await loadTeamDraft("group-a");
  edit("group-a","first");const first=useTeamComposerStore.getState().drafts["group-a"];
  let release!:()=>void;
  pause=new Promise<void>(resolve=>{release=resolve;});
  const admitted=new Promise<void>(resolve=>{entered=resolve;});
  const writing=flushTeamDraft("group-a");await admitted;
  edit("group-a","edited during save");const second=useTeamComposerStore.getState().drafts["group-a"];
  assert.notEqual(first.requestId,second.requestId,"an actual edit gets a new submission identity");
  pause=undefined;entered=undefined;release();await writing;
  assert.equal(saved.get("group-a")?.draft.text,"edited during save");
  assert.deepEqual(requests.map(request=>request.expectedRevision),[0,1],"a delayed save cannot overwrite the later input or reuse its old revision");
  clearSubmittedTeamDraft("group-a",first);
  assert.equal(useTeamComposerStore.getState().drafts["group-a"],second,"a late accepted submission cannot erase the newer input");

  await loadTeamDraft("group-b");edit("group-b","do not duplicate");loseResponse=true;
  await assert.rejects(flushTeamDraft("group-b"),/lost response/);
  assert.equal(useTeamComposerStore.getState().drafts["group-b"].text,"do not duplicate");
  // Explicit reload represents the same persisted snapshot after a restart.
  await loadTeamDraft("group-b",true);
  assert.equal(useTeamComposerStore.getState().drafts["group-b"].requestId,saved.get("group-b")?.draft.requestId);

  let finish!:()=>void;
  const admittedSend=new Promise<void>(resolve=>{finish=resolve;});
  const submission=trackTeamSubmission("group-a",async()=>{await admittedSend;clearSubmittedTeamDraft("group-a",second);});
  let exited=false;const quitting=flushAllTeamDrafts().then(()=>{exited=true;});
  await Promise.resolve();assert.equal(exited,false,"quit joins admitted Team submissions");
  finish();await submission;await quitting;
  assert.equal(saved.get("group-a")?.draft.text,"","quit persists the accepted submission's cleared input");
  resumeTeamDraftEditing();
  // A cancelled quit can finish after a newer quit has frozen the same draft.
  edit("group-a","retained across quit retry");
  let unblockTask!:()=>void, unblockSave!:()=>void;
  const taskGate=new Promise<void>(resolve=>{unblockTask=resolve;});
  const tracked=trackTeamSubmission("group-a",()=>taskGate);
  const oldQuit=flushAllTeamDrafts();
  const superseded=assert.rejects(oldQuit,/superseded/);
  resumeTeamDraftEditing();
  pause=new Promise<void>(resolve=>{unblockSave=resolve;});
  const saving=new Promise<void>(resolve=>{entered=resolve;});
  const newQuit=flushAllTeamDrafts();
  unblockTask();await tracked;await superseded;await saving;
  edit("group-a","must remain blocked during the new quit");
  assert.equal(useTeamComposerStore.getState().drafts["group-a"].text,"retained across quit retry");
  pause=undefined;entered=undefined;unblockSave();await newQuit;
  resumeTeamDraftEditing();
  console.log("PASS Team draft concurrent saves, stale clears, durable submission identity and quit join");
} finally {resumeTeamDraftEditing();dom.window.close();}
