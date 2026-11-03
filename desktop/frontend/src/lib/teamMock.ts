import type { GroupLine, SessionRef, TeamAgentView, TeamGroupHistoryPage, TeamGroupDraftView, TeamMutationResult, TeamPresetDefinition, TeamView } from "../generated/desktopContract.generated";
import type { AgentPresetsView } from "./agentPresets";
import type { TeamHostBindings } from "./teamBindings";

type Ports={presets:()=>Promise<AgentPresetsView>;createSession:(agentID:string)=>Promise<SessionRef>};
export function makeMockTeamBindings(ports:Ports):TeamHostBindings {
  const view:TeamView={roster:{schemaVersion:1,revision:0,agents:[],groups:[],retiredAgents:[],retiredGroups:[]},writable:true,seats:[],groupStatus:[],agentActivity:[]};
  const drafts=new Map<string,TeamGroupDraftView>();
  const read=new Map<string,number>();
  const receipts=new Map<string,{request:string;target:string}>();
  const logs=new Map<string,GroupLine[]>();
  const snapshots=new Map<string,{group:string;lines:GroupLine[]}>();
  let identity=0;
  const clone=()=>structuredClone(view);
  const result=(outcome:string,targetId="",reason="",message=""):TeamMutationResult=>({outcome,targetId,reason,message,view:clone()});
  const check=(key:string,revision:number,request:unknown):TeamMutationResult|null=>{
    const old=receipts.get(key);
    if(old)return old.request===JSON.stringify(request)?result("applied",old.target):result("rejected","","request_reused");
    if(revision!==view.roster.revision)return result("rejected","","conflict");
    if(key.length<8)return result("rejected","","invalid");
    return null;
  };
  const commit=(key:string,request:unknown,target:string)=>{receipts.set(key,{request:JSON.stringify(request),target});view.roster.revision++;return result("applied",target);};
  const prepare=async(agentID:string)=>{
    const member=view.roster.agents.find(member=>member.id===agentID);
    if(!member)throw new Error("no such agent");
    if(member.sessionPath)return {hostId:"local",sessionId:member.sessionPath.slice("session-id:".length)};
    const ref=await ports.createSession(agentID);
    member.sessionPath=`session-id:${ref.sessionId}`;view.roster.revision++;
    return ref;
  };
  return {
    ArchiveTeamAgent:async request=>{
      const prior=check(request.requestId,request.expectedRevision,{...request,action:"archive"});if(prior)return prior;
      const member=view.roster.agents.find(a=>a.id===request.agentId);if(!member)return result("rejected","","target_invalid");
      if(member.main&&!view.roster.agents.some(a=>a.main&&a.id!==member.id))return result("rejected","","invalid");
      view.roster.retiredAgents.push({...member,archived:true});view.roster.agents=view.roster.agents.filter(a=>a.id!==member.id);
      view.roster.revision++;receipts.set(request.requestId,{request:JSON.stringify({...request,action:"archive"}),target:member.id});return result("applied",member.id);
    },
    RestoreTeamAgent:async request=>{
      const prior=check(request.requestId,request.expectedRevision,{...request,action:"restore"});if(prior)return prior;
      const member=view.roster.retiredAgents.find(a=>a.id===request.agentId&&a.archived);if(!member)return result("rejected","","target_invalid");
      view.roster.agents.push({...member,archived:false});view.roster.retiredAgents=view.roster.retiredAgents.filter(a=>a.id!==member.id);
      view.roster.revision++;receipts.set(request.requestId,{request:JSON.stringify({...request,action:"restore"}),target:member.id});return result("applied",member.id);
    },
    BrowserTeamAgentForTab:async()=>"",
    GetTeam:async()=>{view.groupStatus=view.roster.groups.map(group=>({groupId:group.id,lastSequence:logs.get(group.id)?.length??0,readSequence:read.get(group.id)??0,unread:(logs.get(group.id)??[]).filter(line=>line.seq>(read.get(group.id)??0)&&line.from!=="user").length}));return clone();},
    ReadTeamGroupDraft:async group=>structuredClone(drafts.get(group)??{revision:0,draft:{requestId:"",text:"",mentions:[],everyone:false}}),
    SaveTeamGroupDraft:async request=>{
      const previous=drafts.get(request.groupId);
      if((previous?.revision??0)!==request.expectedRevision)return {outcome:"rejected",reason:"conflict",view:structuredClone(previous!)};
      const next={revision:request.expectedRevision+1,draft:structuredClone(request.draft)};drafts.set(request.groupId,next);return {outcome:"applied",view:structuredClone(next)};
    },
    MarkTeamGroupRead:async(group,snapshot,sequence)=>{if(snapshots.get(snapshot)?.group!==group)throw new Error("stale_cursor");read.set(group,Math.max(read.get(group)??0,sequence));},
    SaveTeamAgent:async request=>{
      const prior=check(request.requestId,request.expectedRevision,request);if(prior)return prior;
      if(!request.settings.name.trim())return result("rejected","","invalid");
      let member=view.roster.agents.find(member=>member.id===request.id);
      if(request.id&&!member)return result("rejected","","target_invalid");
      let preset=request.settings.preset;
      if(request.presetId){
        const p=(await ports.presets()).presets.find(p=>p.id===request.presetId);
        if(!p)return result("rejected","","invalid","Agent preset not found");
        preset={id:p.id,name:p.name,description:p.description,body:p.body,icon:p.icon??"",iconImage:p.iconImage??"",iconColor:p.iconColor??"",keepCoding:p.keepCoding,complete:p.complete,includeRuntimeContext:p.includeRuntimeContext,restrictTools:p.tools!==null,tools:p.tools??[]} satisfies TeamPresetDefinition;
      }
      const otherMain=view.roster.agents.some(agent=>agent.id!==request.id&&agent.main);
      if(member?.main&&!request.settings.main&&!otherMain)return result("rejected","","invalid","The team needs at least one main agent");
      if(!member){member={...structuredClone(request.settings),id:`a-${(++identity).toString(16).padStart(16,"0")}`,sessionPath:"",createdAt:new Date().toISOString(),createdBy:"user"} satisfies TeamAgentView;view.roster.agents.push(member);}
      Object.assign(member,structuredClone(request.settings),{preset:structuredClone(preset)});
      if(view.roster.agents.length===1)member.main=true;
      return commit(request.requestId,request,member.id);
    },
    SaveTeamGroup:async request=>{
      const prior=check(request.requestId,request.expectedRevision,request);if(prior)return prior;
      if(request.settings.members.length<2||request.settings.members.some(id=>!view.roster.agents.some(member=>member.id===id)))return result("rejected","","invalid");
      let group=view.roster.groups.find(group=>group.id===request.id);
      if(request.id&&!group)return result("rejected","","target_invalid");
      if(!group){group={...structuredClone(request.settings),id:`g-${(++identity).toString(16).padStart(16,"0")}`,createdAt:new Date().toISOString(),createdBy:"user"};view.roster.groups.push(group);}
      Object.assign(group,structuredClone(request.settings));
      return commit(request.requestId,request,group.id);
    },
    PrepareTeamAgentSession:prepare,
    DeleteTeamTarget:async request=>{
      const prior=check(request.requestId,request.expectedRevision,request);if(prior)return prior;
      if(request.agentId){
        const member=view.roster.agents.find(member=>member.id===request.agentId);
        if(!member)return result("rejected","","target_invalid");
        if(member.main&&view.roster.agents.length>1&&!view.roster.agents.some(other=>other.id!==member.id&&other.main))return result("rejected","","invalid");
        view.roster.retiredAgents.push(member);view.roster.agents=view.roster.agents.filter(other=>other.id!==member.id);
        view.roster.groups=view.roster.groups.filter(group=>{
          if(!group.members.includes(member.id))return true;
          if(group.members.length<=2){view.roster.retiredGroups.push({...group,archived:true});return false;}
          group.members=group.members.filter(id=>id!==member.id);if(group.admin===member.id){group.admin="";group.mode="everyone";}return true;
        });
      }else{
        const group=view.roster.groups.find(group=>group.id===request.groupId);
        if(!group)return result("rejected","","target_invalid");
        view.roster.retiredGroups.push({...group,archived:true});view.roster.groups=view.roster.groups.filter(other=>other.id!==group.id);
      }
      return commit(request.requestId,request,request.agentId||request.groupId);
    },
    ReorderTeam:async request=>{
      const prior=check(request.requestId,request.expectedRevision,request);if(prior)return prior;
      const agents=request.agentIds.map(id=>view.roster.agents.find(agent=>agent.id===id));
      const groups=request.groupIds.map(id=>view.roster.groups.find(group=>group.id===id));
      if(agents.length!==view.roster.agents.length||groups.length!==view.roster.groups.length||agents.some(a=>!a)||groups.some(g=>!g)||new Set(request.agentIds).size!==agents.length||new Set(request.groupIds).size!==groups.length)return result("rejected","","invalid");
      view.roster.agents=agents.filter((a):a is TeamAgentView=>!!a);view.roster.groups=groups.filter((g):g is NonNullable<typeof g>=>!!g);
      return commit(request.requestId,request,"");
    },
    OpenTeamAgent:async()=>{throw new Error("Use the canonical Team session navigation");},
    SendTeamGroupMessage:async request=>{
      const group=view.roster.groups.find(group=>group.id===request.groupId);
      if(!group||group.archived)return {outcome:"rejected",reason:"target_invalid"};
      if(request.mentions.some(id=>!group.members.includes(id)))return {outcome:"rejected",reason:"target_invalid"};
      const old=receipts.get(request.requestId);
      const lines=logs.get(group.id)??[];
      if(old)return old.request===JSON.stringify(request)?{outcome:"accepted",line:lines.find(line=>line.requestId===request.requestId)}:{outcome:"rejected",reason:"request_reused"};
      const line:GroupLine={seq:lines.length+1,at:new Date().toISOString(),from:"user",kind:"post",text:request.text,mentions:request.mentions,requestId:request.requestId};
      logs.set(group.id,[...lines,line]);receipts.set(request.requestId,{request:JSON.stringify(request),target:group.id});
      return {outcome:"accepted",line};
    },
    ReadTeamGroupHistory:async(group,cursor)=>{
      let token="",position=0,newer=false;
      if(cursor){try{const parsed=JSON.parse(cursor) as {token:string;position:number;newer:boolean};token=parsed.token;position=parsed.position;newer=parsed.newer;}catch{return stale();}}
      else {token=crypto.randomUUID();if(snapshots.size>=64){const oldest=snapshots.keys().next().value;if(oldest)snapshots.delete(oldest);}snapshots.set(token,{group,lines:[...(logs.get(group)??[])]});}
      const snapshot=snapshots.get(token);
      if(!snapshot||snapshot.group!==group)return stale();
      const available=snapshot.lines.filter(line=>(!position||newer)?line.seq>position:line.seq<position);
      const lines=position&&newer?available.slice(0,32):available.slice(-32);
      const first=lines[0]?.seq??0,last=lines[lines.length-1]?.seq??0,max=snapshot.lines.length;
      return {status:"ready",lines,snapshot:token,maxSequence:max,olderCursor:first>1?JSON.stringify({token,position:first,newer:false}):"",newerCursor:last<max?JSON.stringify({token,position:last,newer:true}):""};
    },
    ContinueTeamGroup:async()=>{},StopTeamGroup:async()=>{},StopTeamSeat:async()=>{},
  };
}
function stale():TeamGroupHistoryPage{return {status:"stale_cursor",lines:[],olderCursor:"",newerCursor:"",snapshot:"",maxSequence:0};}
