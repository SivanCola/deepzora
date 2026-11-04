import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import TeamPermissionChoice from "../components/TeamPermissionChoice";
import type { SaveTeamAgentRequest } from "../generated/desktopContract.generated";
import { LocaleProvider } from "../lib/i18n";
import { memberSettings } from "../lib/teamAgentSettings";
import { makeMockTeamBindings } from "../lib/teamMock";
import { ToastProvider } from "../lib/toast";
import { installBridgeApp, installDom } from "./composerInboxHarness";

const dom = installDom();
globalThis.HTMLInputElement = dom.window.HTMLInputElement;
const team = makeMockTeamBindings({ presets: async () => ({current:"default",locked:false,presets:[],problems:[]}), createSession: async id => ({hostId:"local",sessionId:id}) });
for (const name of ["Ada", "Ben"]) {
  const view = await team.GetTeam();
  await team.SaveTeamAgent({id:"",requestId:`create-${name}`,expectedRevision:view.roster.revision,settings:{...memberSettings(),name,instructions:`${name} instructions`,initialModel:"fixture/model"}});
}
const [ada,ben] = (await team.GetTeam()).roster.agents;
const requests: SaveTeamAgentRequest[] = [];
let conflict = false, release: (()=>void)|undefined, hold = false;
installBridgeApp({ ...team, SaveTeamAgent: async (request: SaveTeamAgentRequest) => {
  requests.push(request);
  if (hold) await new Promise<void>(resolve => { release = resolve; });
  if (conflict) {
    conflict = false;
    const view = await team.GetTeam();
    await team.SaveTeamAgent({id:ada.id,requestId:"concurrent-edit",expectedRevision:view.roster.revision,settings:{...memberSettings(view.roster.agents[0]),instructions:"new instructions"}});
  }
  return team.SaveTeamAgent(request);
} });
const root = createRoot(document.getElementById("root")!);
async function render(id: string) {
  await act(async()=>root.render(<LocaleProvider><ToastProvider><TeamPermissionChoice key={id} agentId={id} disabled={false}/></ToastProvider></LocaleProvider>));
}
async function wait(check:()=>boolean) {
  for(let i=0;i<100&&!check();i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,10));});
  assert.ok(check());
}
const trigger=()=>document.querySelector<HTMLButtonElement>(".composer-choice")!;
async function choose(value:string) {
  await wait(()=>!!trigger()&&!trigger().disabled);
  await act(async()=>trigger().click());
  await act(async()=>document.querySelector<HTMLButtonElement>(`[data-value="${value}"]`)!.click());
}
try {
  await render(ada.id);
  await choose("workspace-write");
  await wait(()=>trigger().textContent?.includes("Workspace access")===true&&!trigger().disabled);
  assert.equal(requests[0].id,ada.id);
  assert.equal(requests[0].settings.instructions,"Ada instructions");
  assert.equal(requests[0].settings.initialModel,"fixture/model");
  assert.equal((await team.GetTeam()).roster.agents[1].permission,"read-only");

  await choose("danger-full-access");
  await wait(()=>!!document.querySelector('[role="dialog"]'));
  assert.match(document.querySelector('[role="dialog"]')!.textContent!,/every private and group chat/);
  assert.equal(requests.length,1,"opening confirmation does not save");
  assert.ok(document.querySelector<HTMLButtonElement>(".risk-confirmation__confirm")!.disabled);
  await act(async()=>document.querySelector<HTMLButtonElement>(".risk-confirmation__cancel")!.click());
  assert.equal(requests.length,1);
  await choose("danger-full-access");
  await act(async()=>document.querySelector<HTMLInputElement>('.risk-confirmation input[type="checkbox"]')!.click());
  await act(async()=>document.querySelector<HTMLButtonElement>(".risk-confirmation__confirm")!.click());
  await wait(()=>requests.length===2&&!trigger().disabled);
  assert.equal((await team.GetTeam()).roster.agents[0].permission,"danger-full-access");

  conflict=true;
  await choose("read-only");
  await wait(()=>requests.length===3&&!trigger().disabled);
  assert.match(document.body.textContent!,/Team settings changed/);
  assert.equal(requests.length,3,"conflict is never automatically retried");
  const current=(await team.GetTeam()).roster.agents[0];
  assert.equal(current.permission,"danger-full-access");
  assert.equal(current.instructions,"new instructions");

  hold=true;
  await choose("read-only");
  await wait(()=>!!release);
  assert.ok(trigger().disabled,"disable repeated submissions while saving");
  await render(ben.id);
  await wait(()=>!!trigger()&&!trigger().disabled);
  await act(async()=>release!());
  await wait(()=>!requests.some(request=>request.id===ben.id));
  assert.equal((await team.GetTeam()).roster.agents[0].permission,"read-only");
  assert.equal((await team.GetTeam()).roster.agents[1].permission,"read-only");
  assert.equal(requests.at(-1)!.id,ada.id,"late response cannot change the newly displayed member");
  await render("deleted-member");
  await wait(()=>!!trigger()&&trigger().disabled);
  console.log("PASS Team permission choice: member save, full-access acknowledgement, CAS conflict, pending save, navigation and invalid member");
} finally { await act(async()=>root.unmount()); dom.window.close(); }
