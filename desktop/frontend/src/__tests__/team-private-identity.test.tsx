import assert from "node:assert/strict";
import React, {act} from "react";
import {createRoot} from "react-dom/client";
import {installDom} from "./composerInboxHarness";
import {installDesktopHostStub} from "./desktopHostStub";
import {useTeamSpeaker} from "../lib/useTeamSpeaker";
import {LocaleProvider, useT, t} from "../lib/i18n";
import {TeamPrivateHeader} from "../components/TeamPrivateIdentity";
import {buildTopicbarView} from "../app-shell/TopicbarActionsStack";
import {TopicbarRegion} from "../app-shell/TopicbarRegion";
import {buildSidebarRegionProps} from "../app-shell/chromeRegionBuilders";
import type {TeamView, TeamAgentView} from "../generated/desktopContract.generated";
import type {TabMeta} from "../lib/types";

const dom=installDom();
const root=createRoot(document.getElementById("root")!);
const member=(id:string,name:string):TeamAgentView=>({id,name,role:"",instructions:"",avatar:{glyph:"bot"},folder:"",initialModel:"",permission:"read-only",main:false,pinned:false,hidden:false,sessionPath:`session-id:desktop-team-${id}`,createdBy:"user",createdAt:"2026-11-04T00:00:00Z"});
const ada=member("a-0000000000000001","Ada With Spaces"),ben=member("a-0000000000000002","Ben");
const roster=(agents:TeamAgentView[]):TeamView=>({roster:{schemaVersion:1,revision:1,agents,groups:[],retiredAgents:[],retiredGroups:[],prefs:{memoryReview:false}},writable:true,seats:[],groupStatus:[],agentActivity:[]});
let pending:((view:TeamView)=>void)[]=[];
const host=installDesktopHostStub({GetTeam:()=>new Promise<TeamView>(resolve=>pending.push(resolve))});
const noop=()=>{};
function Surface({tab}:{tab:TabMeta}) {
  const translate=useT();
  const speaker=useTeamSpeaker(tab.teamAgentId);
  const displayed=speaker??{id:tab.teamAgentId!,name:translate("team.member")};
  return <><TeamPrivateHeader member={{...displayed,onManage:noop}}/><TopicbarRegion view={buildTopicbarView({
    t:translate,locale:"en",activeTab:tab,cwd:tab.workspaceRoot,teamMemberName:displayed.name,
    chromeHidden:false,windowsBrand:false,automationReturn:false,sidebar:{title:"Sidebar",blocked:false,pressed:false,collapsed:false},rename:{editing:false,draft:""},
  })} commands={{openAutomation:noop,toggleSidebar:noop,setTitleDraft:noop,commitRename:noop,cancelRename:noop,startRename:noop,openWorktree:noop}}>{null}</TopicbarRegion></>;
}
const tab=(agent:TeamAgentView)=>({id:`tab-${agent.id}`,teamAgentId:agent.id,scope:"project",workspaceRoot:`/team/workspaces/${agent.id}`,topicId:`team-desktop-team-${agent.id}`,topicTitle:`team-desktop-team-${agent.id}`,sessionPath:agent.sessionPath,session:{hostId:"local",sessionId:`desktop-team-${agent.id}`}} as TabMeta);
const render=async(agent:TeamAgentView)=>act(async()=>root.render(<LocaleProvider><Surface tab={tab(agent)}/></LocaleProvider>));
const flush=async()=>act(async()=>{await Promise.resolve();});
function assertName(name:string) {
  assert.equal(document.querySelector(".team-private-header strong")?.textContent,name);
  assert.equal(document.querySelector(".topicbar h1")?.textContent,name);
  assert.equal(document.querySelector(".topicbar h1")?.getAttribute("title"),name);
  assert.equal(document.querySelector(".topicbar__workspace-label")?.textContent,"Private chat");
  assert.equal(document.querySelector(".topicbar__title-button"),null,"member title is edited through member settings");
  assert.ok(!document.body.textContent?.includes("a-000000"),"internal IDs never become display text");
}
try {
  await render(ada);
  assert.equal(pending.length,1);
  await render(ben);
  await act(async()=>pending.shift()!(roster([ada,ben])));
  assertName("Ben");
  await render(ada);assertName("Ada With Spaces");
  await render(ben);assertName("Ben");
  await act(async()=>host.emit("team:changed"));
  const stale=pending.shift()!;
  await act(async()=>host.emit("team:changed"));
  const latest=pending.shift()!;
  await act(async()=>latest(roster([ada,{...ben,name:"Ben Renamed"}])));
  assertName("Ben Renamed");
  await act(async()=>stale(roster([ada,ben])));await flush();assertName("Ben Renamed");
  await render(ada);assertName("Ada With Spaces");

  // Only canonical ordinary sessions may synthesize a missing project row.
  const input={className:"",shell:{sidebarCollapsed:false,page:{kind:"workspace"}},t,geometry:{sidebarResizeMinWidth:200,sidebarRenderWidth:260,startSidebarResize:noop,resizeSidebarWithKeyboard:noop,setExpandedSidebarWidth:noop},
    projectTree:{activeTab:tab(ada),refreshSignal:0,searchExpanded:false,searchFocusSignal:0,showShortcutBadges:false,shortcutPlatform:"mac",onVisibleTopicsChange:noop},topics:{onCreateTopic:noop,onCreateIsolatedWorktree:noop,refreshProjectsAndTabs:noop,renameTopic:noop,onAddProject:noop},commands:{onNewSession:noop,onOpenPalette:noop,onOpenTrash:noop,onOpenAutomation:noop,onOpenSettings:noop,onOpenTopic:noop},paletteShortcut:""} as unknown as Parameters<typeof buildSidebarRegionProps>[0];
  const teamSidebar=buildSidebarRegionProps(input);
  assert.equal(teamSidebar.projectTree.activeSession,undefined,"filtered Team history must not reappear as a project session");
  assert.equal(teamSidebar.projectTree.activeSessionPath,ada.sessionPath,"Team sidebar still identifies the selected member");
  const ordinary={...tab(ada),teamAgentId:undefined};
  assert.deepEqual(buildSidebarRegionProps({...input,projectTree:{...input.projectTree,activeTab:ordinary}}).projectTree.activeSession,ordinary.session,"ordinary missing-session hydration remains available");
  console.log("PASS Team host identity switches, late roster fencing, rename, private titles and ordinary-project hydration exclusion");
} finally {await act(async()=>root.unmount());host.uninstall();dom.window.close();}
