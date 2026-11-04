import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";
import { makeSessionUIMock } from "../lib/sessionUIMock";
import { makeMockTeamBindings } from "../lib/teamMock";
import { memberSettings } from "../lib/teamAgentSettings";

const dom = installDom();
globalThis.HTMLInputElement = dom.window.HTMLInputElement;
const models: string[] = [], managed: string[] = [], permissionChanges: string[] = [], sends: string[] = [];
const catalogCalls: string[] = [];
const team = makeMockTeamBindings({ presets: async () => ({ current:"default",locked:false,presets:[],problems:[] }), createSession: async id => ({hostId:"local",sessionId:id}) });
for (const name of ["Ada With Spaces", "Ben"]) {
  const view = await team.GetTeam();
  await team.SaveTeamAgent({id:"",requestId:`create-${name}`,expectedRevision:view.roster.revision,settings:{...memberSettings(),name}});
}
installBridgeApp({
  ...makeSessionUIMock(async () => {}),
  ...team,
  ModelsForTab: async (tab: string) => {
    catalogCalls.push(tab);
    return [{ref:"fixture/fast",provider:"fixture",model:"fast",current:true},{ref:"fixture/careful",provider:"fixture",model:"careful",current:false}];
  },
  ListAgentPresets: async () => ({current:"default",locked:false,presets:[],problems:[]}),
});
const workspaceContext = {
  scope: "project" as const, workspaceRoot: "/private/team/workspaces/a-internal", workspaceName: "internal-workspace-title",
  scopeKey: "team-workspace", onSwitchWorkspace: async () => {}, onWorkWithoutProject: async () => {}, onRefreshProjects: async () => {},
};
const member = {id:"a-0000000000000001",name:"Ada With Spaces",glyph:"bot",color:"#ffcc00",onManage:()=>managed.push("ada")};
const {root, rerender} = await renderComposer({
  tabId:"ada-tab",sessionKey:"ada-session",composerTarget:{kind:"session",tabId:"ada-tab"},
  formalSessionRef:{hostId:"local",sessionId:"ada-session"}, teamMember:member,workspaceContext,
  modelLabel:"fast",heroMode:true,onSend:text=>{sends.push(text);},
  onSwitchModel:async model=>{models.push(model);return true;},onSetToolApprovalMode:mode=>permissionChanges.push(mode),
});
async function waitFor(check:()=>boolean,label:string) {
  for(let i=0;i<100&&!check();i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,10));});
  assert.ok(check(),label);
}
function assertIdentity(name:string) {
  const input=document.querySelector<HTMLTextAreaElement>("textarea.composer__input:not([aria-hidden=true])")!;
  assert.equal(input.placeholder,`Message ${name}…`);
  assert.equal(input.getAttribute("aria-label"),`Message ${name}…`);
  assert.equal(document.querySelector(".team-private-header strong")?.textContent,name);
  assert.equal(document.querySelector(".composer-meta__control--agent-preset"),null,"private members do not offer ordinary session presets");
  assert.ok(!document.body.textContent?.includes("internal-workspace-title"),"workspace picker cannot replace a member's execution directory");
  assert.ok(!document.body.textContent?.includes("a-0000000000000001"),"stable identity is not a display title");
}
try {
  assertIdentity(member.name);
  for(const heroMode of [true,false]) {
    await rerender({heroMode});
    assertIdentity(member.name);
    await waitFor(()=>!!document.querySelector<HTMLButtonElement>(".modelsw__trigger:not(:disabled)"),"member session model remains selectable");
    await act(async()=>document.querySelector<HTMLButtonElement>(".modelsw__trigger")!.click());
    await waitFor(()=>document.querySelectorAll('[role="option"]').length===2,"model choices appear");
    await act(async()=>document.querySelector<HTMLButtonElement>('[role="option"][aria-selected="false"]')!.click());
    assert.equal(models.at(-1),"fixture/careful");
    if(!heroMode) {
      await waitFor(()=>!!document.querySelector(".composer-meta__control--approval button:not(:disabled)"),"member permission is editable");
      await act(async()=>document.querySelector<HTMLButtonElement>(".composer-meta__control--approval button")!.click());
      await act(async()=>document.querySelector<HTMLButtonElement>('[data-value="workspace-write"]')!.click());
      await waitFor(()=>document.querySelector(".composer-meta__control--approval")?.textContent?.includes("Workspace access")===true,"saved member permission is displayed");
      assert.equal((await team.GetTeam()).roster.agents[0].permission,"workspace-write");
      assert.deepEqual(permissionChanges,[],"member edits never use the ordinary session permission endpoint");
    }
  }
  assert.ok(catalogCalls.every(tab=>tab==="ada-tab"),"model queries use the displayed host tab");
  await act(async()=>document.querySelector<HTMLButtonElement>('[aria-label="Member settings"]')!.click());
  assert.deepEqual(managed,["ada"]);
  await rerender({teamMember:{...member,name:"Ada Renamed"}});
  assertIdentity("Ada Renamed");
  await rerender({tabId:"ben-tab",sessionKey:"ben-session",composerTarget:{kind:"session",tabId:"ben-tab"},formalSessionRef:{hostId:"local",sessionId:"ben-session"},teamMember:{id:"a-0000000000000002",name:"Ben",onManage:()=>managed.push("ben")}});
  assertIdentity("Ben");
  await act(async()=>{
    const event=new window.Event("paste",{bubbles:true,cancelable:true});
    Object.defineProperty(event,"clipboardData",{value:{files:[],items:[],types:["text/plain"],getData:()=>"Private message for Ben"}});
    document.querySelector("textarea.composer__input:not([aria-hidden=true])")!.dispatchEvent(event);
  });
  await waitFor(()=>!!document.querySelector<HTMLButtonElement>(".composer__btn--send:not(:disabled)"),"member composer accepts text");
  await act(async()=>document.querySelector<HTMLButtonElement>(".composer__btn--send")!.click());
  assert.deepEqual(sends,["Private message for Ben"]);
  await rerender({teamMember:undefined,workspaceContext:undefined});
  await waitFor(()=>!!document.querySelector(".composer-meta__control--agent-preset"),"ordinary chat keeps its preset control");
  assert.equal(document.querySelector(".team-private-header"),null);
  console.log("PASS Team private composer: empty/message chrome, identity rename/switch, sending, session model selection and member-owned permissions");
} finally {
  await act(async()=>root.unmount());dom.window.close();
}
