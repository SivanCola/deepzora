import assert from "node:assert/strict";
import test from "node:test";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import type { SaveTeamScheduleRequest, SaveTeamSecretRequest, Schedule, TeamScheduleResult, TeamSecretResult } from "../generated/desktopContract.generated";
import { TeamScheduleSettings } from "../components/TeamScheduleSettings";
import { TeamSecretSettings } from "../components/TeamSecretSettings";
import { SettingsExperienceProvider } from "../components/SettingsExperience";
import { LocaleProvider } from "../lib/i18n";
import { memberSettings } from "../lib/teamAgentSettings";
import { makeMockTeamBindings } from "../lib/teamMock";
import { installDom, installBridgeApp } from "./composerInboxHarness";

const dom=installDom();
globalThis.HTMLInputElement=dom.window.HTMLInputElement;
const bindings=makeMockTeamBindings({presets:async()=>({current:"default",locked:false,presets:[],problems:[]}),createSession:async id=>({hostId:"local",sessionId:id})});
await bindings.SaveTeamAgent({id:"",expectedRevision:0,requestId:"recovery-create-agent",settings:{...memberSettings(),name:"Ada"}});
const team=await bindings.GetTeam(),agent=team.roster.agents[0];
const root=createRoot(document.getElementById("root")!);
const wrap=(child:React.ReactNode)=><LocaleProvider><SettingsExperienceProvider>{child}</SettingsExperienceProvider></LocaleProvider>;
const render=async(child:React.ReactNode)=>{await act(async()=>root.render(wrap(child)));};
async function settle(){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function button(text:string){const found=[...document.querySelectorAll<HTMLButtonElement>("button")].find(item=>item.textContent===text);assert.ok(found,`missing ${text}`);return found;}
async function click(text:string){await act(async()=>button(text).click());}
async function change(element:HTMLInputElement|HTMLTextAreaElement,value:string){
  // Invoke React's input boundary so this suite isolates async draft ownership;
  // the browser acceptance suite exercises actual native input events.
  const key=Object.keys(element).find(key=>key.startsWith("__reactProps$"))!;
  const props=(element as unknown as Record<string,{onChange:(event:unknown)=>void}>)[key];
  await act(async()=>props.onChange({target:{value}}));
}
const submit=async()=>{await act(async()=>document.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));};

try {
 await test("Team schedule conflicts require review, retain drafts and preserve CAS",async()=>{
  let revision=1,rosterRevision=team.roster.revision,failReview=false;
  let schedule:Schedule={id:"s-0000000000000001",name:"Daily",agent:agent.id,prompt:"original task",repeat:"daily",timezone:"UTC",clock:"09:00",days:[],anchor:"2026-11-05T09:00:00Z",enabled:true,next:"2026-11-05T09:00:00Z",lastAt:"0001-01-01T00:00:00Z"};
  const requests:SaveTeamScheduleRequest[]=[];
  installBridgeApp({...bindings,GetTeam:async()=>({...structuredClone(team),roster:{...structuredClone(team.roster),revision:rosterRevision}}),GetTeamSchedules:async()=>{
    if(failReview)throw new Error("review temporarily unavailable");
    return {revision,schedules:[structuredClone(schedule)]};
  },SaveTeamSchedule:async(request:SaveTeamScheduleRequest):Promise<TeamScheduleResult>=>{
    requests.push(request);
    if(request.expectedRevision!==revision||request.expectedRosterRevision!==rosterRevision)return {outcome:"rejected",reason:"conflict",view:{revision,schedules:[schedule]}};
    schedule=request.schedule;revision++;
    return {outcome:"applied",view:{revision,schedules:[schedule]}};
  }});
  await render(<TeamScheduleSettings team={team}/>);await settle();await click("Edit");
  await change(document.querySelector<HTMLInputElement>("form input")!,"My retained name");
  await change(document.querySelector<HTMLTextAreaElement>("form textarea")!,"My retained task");
  revision=2;rosterRevision++;
  schedule={...schedule,name:"Changed elsewhere",prompt:"Saved task to review",clock:"10:00",lastOutcome:"delivered"};
  await submit();
  assert.equal(requests.length,1,"a conflict never automatically retries");
  assert.match(document.body.textContent!,/team changed while you were editing/);
  failReview=true;await click("Review latest saved version");
  assert.match(document.body.textContent!,/review temporarily unavailable/);
  assert.equal(document.querySelector<HTMLInputElement>("form input")!.value,"My retained name");
  failReview=false;await click("Review latest saved version");
  assert.ok(document.body.textContent!.includes("Saved task to review"),"show the saved values before allowing overwrite");
  assert.equal(button("Save").disabled,true,"review does not silently rebase or save");
  await submit();assert.equal(requests.length,1,"submit cannot bypass the review step");
  await click("Keep my draft for the next save");
  assert.equal(requests.length,1);
  assert.equal(document.querySelector<HTMLInputElement>("form input")!.value,"My retained name");
  assert.equal(document.querySelector<HTMLTextAreaElement>("form textarea")!.value,"My retained task");
  // A further scheduler update still wins CAS; reviewing is not a bypass.
  revision=3;await submit();assert.equal(requests.length,2);
  assert.equal(requests[1].expectedRevision,2);
  assert.equal(requests[1].expectedRosterRevision,rosterRevision);
  assert.notEqual(requests[1].requestId,requests[0].requestId);
  await click("Review latest saved version");await click("Keep my draft for the next save");
  await submit();
  assert.equal(requests.length,3);
  assert.equal(requests[2].expectedRevision,3);
  assert.equal(schedule.name,"My retained name");
  assert.equal(schedule.prompt,"My retained task");
  assert.equal(schedule.lastOutcome,"delivered","retain host runtime fields from the reviewed snapshot");
  assert.equal(document.querySelector("form"),null);
 });

 await test("Team secret saves freeze all inputs and retain failed drafts",async()=>{
  await render(<div/>);
  const secret={name:"SERVICE_KEY",purpose:"old purpose",agents:[agent.id],updatedAt:"2026-11-04T00:00:00Z"};
  const view={available:true,revision:1,rosterRevision:team.roster.revision,secrets:[secret],requests:[],retiredCount:0,pendingCleanup:0};
  let finishSave:((result:TeamSecretResult)=>void)|undefined;
  const secretRequests:SaveTeamSecretRequest[]=[];
  installBridgeApp({...bindings,GetTeamSecrets:async()=>structuredClone(view),SaveTeamSecret:(request:SaveTeamSecretRequest)=>{
    secretRequests.push(request);
    return new Promise<TeamSecretResult>(resolve=>{finishSave=resolve;});
  }});
  await render(<TeamSecretSettings team={team}/>);await settle();await click("Edit");
  let inputs=document.querySelectorAll<HTMLInputElement>("form input");
  await change(inputs[1],"retained purpose");await change(inputs[2],"disposable-secret-draft-8412");
  await submit();
  inputs=document.querySelectorAll<HTMLInputElement>("form input");
  assert.ok([...inputs].every(input=>input.disabled),"all value, description and grant controls are frozen during save");
  await click("Save");await submit();
  assert.equal(secretRequests.length,1,"pending saves cannot be duplicated");
  await act(async()=>finishSave!({outcome:"rejected",reason:"storage_failed",view}));
  inputs=document.querySelectorAll<HTMLInputElement>("form input");
  assert.equal(inputs[1].disabled,false);
  assert.equal(inputs[1].value,"retained purpose");
  assert.equal(inputs[2].value,"disposable-secret-draft-8412","failure preserves the password draft");
  assert.equal(inputs[3].checked,true,"failure preserves member grants");
  await change(inputs[1],"purpose after failed save");
  await submit();
  assert.equal(secretRequests.length,2);
  assert.equal(secretRequests[1].purpose,"purpose after failed save");
  assert.notEqual(secretRequests[1].requestId,secretRequests[0].requestId);
  await act(async()=>finishSave!({outcome:"applied",view}));await settle();
  assert.equal(document.querySelector("form"),null);
  assert.ok(!document.body.textContent!.includes("disposable-secret-draft-8412"));
 });
}finally{await act(async()=>root.unmount());dom.window.close();}
